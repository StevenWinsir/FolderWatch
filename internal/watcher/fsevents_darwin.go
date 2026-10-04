//go:build darwin && cgo

package watcher

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include "fsevents_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"

	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

// FSEvents is recursively rooted by the OS; neither the number of directories
// nor the number of files consumes individual watch descriptors.
type FSEvents struct {
	opts            Options
	mu              sync.Mutex
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	calls           chan reconcileCall
	events          chan RawEvent
	errors          chan error
	incoming        chan nativeEvent
	lost            chan struct{}
	root            string
	identity        fs.FileInfo
	stream          *C.FWEventStream
	handle          cgo.Handle
	dirty           bool
}

type nativeEvent struct {
	path  string
	flags uint32
}

func preferredWatcher(opts Options) (Watcher, error) { return &FSEvents{opts: opts}, nil }

//export folderwatchEvent
func folderwatchEvent(handle C.uintptr_t, path *C.char, flags C.uint32_t) {
	w := cgo.Handle(handle).Value().(*FSEvents)
	// C memory only survives the callback. Copy the path, but never block a
	// system callback waiting for Go/UI. Overflow has a separate retained bit.
	e := nativeEvent{C.GoString(path), uint32(flags)}
	select {
	case w.incoming <- e:
	default:
		select {
		case w.lost <- struct{}{}:
		default:
		}
	}
}

func (w *FSEvents) Start(parent context.Context, root string) (<-chan RawEvent, <-chan error, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, nil, ErrClosed
	}
	if w.started {
		return nil, nil, fmt.Errorf("watcher already started")
	}
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	var err error
	w.root, err = pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return nil, nil, err
	}
	w.identity, err = os.Lstat(w.root)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.calls = make(chan reconcileCall)
	w.events = make(chan RawEvent, w.opts.EventBuffer)
	w.errors = make(chan error, 16)
	w.incoming = make(chan nativeEvent, w.opts.EventBuffer)
	w.lost = make(chan struct{}, 1)
	w.handle = cgo.NewHandle(w)
	name := C.CString(w.root)
	w.stream = C.fw_events_start(name, C.uintptr_t(w.handle))
	C.free(unsafe.Pointer(name))
	if w.stream == nil {
		w.handle.Delete()
		cancel()
		return nil, nil, fmt.Errorf("FSEvents could not start a recursive stream for %q", w.root)
	}
	w.started = true
	go w.loop(ctx)
	return w.events, w.errors, nil
}

func (w *FSEvents) checkRoot() error {
	info, err := os.Lstat(w.root)
	if err != nil || !info.IsDir() || !os.SameFile(info, w.identity) {
		return ErrRootGone
	}
	return nil
}

func (w *FSEvents) warn(err error) {
	select {
	case w.errors <- err:
	default:
		w.dirty = true
	}
}

func (w *FSEvents) translate(e nativeEvent) error {
	// Values are the public CoreServices FSEvents.h flag ABI (10.7+).
	const (
		mustScan    = 0x1 | 0x2 | 0x4 | 0x8
		historyDone = 0x10
		rootChanged = 0x20
		mount       = 0x40 | 0x80
		created     = 0x100
		removed     = 0x200
		renamed     = 0x800
		isDirectory = 0x20000
		isSymlink   = 0x40000
	)
	if e.flags&(mustScan|mount|rootChanged) != 0 {
		if err := w.checkRoot(); err != nil {
			return err
		}
		w.dirty = true
		return nil
	}
	if e.flags == historyDone {
		return nil
	}
	key, err := pathutil.Key(w.root, e.path)
	if err != nil {
		return nil
	} // FSEvents may also report an enclosing scope.
	if key == "." {
		if err := w.checkRoot(); err != nil {
			return err
		}
	}
	isDir := e.flags&isDirectory != 0 && e.flags&isSymlink == 0
	// A path may have become a symlink since delivery; never inspect its target.
	if err := pathutil.CheckParents(w.root, key); err != nil {
		w.dirty = true
		return nil
	}
	if info, err := os.Lstat(e.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		isDir = false
	}
	ignored, err := w.opts.Filter.Match(key, isDir)
	if err != nil {
		w.dirty = true
		return err
	}
	if ignored {
		return nil
	}
	op := Write // all flags invalidate; the resolver, not OS flags, decides content changes
	if e.flags&created != 0 {
		op |= Create
	}
	if e.flags&removed != 0 {
		op |= Remove
	}
	if e.flags&renamed != 0 {
		op |= Rename
	}
	if w.dirty {
		return nil
	}
	select {
	case w.events <- RawEvent{Path: key, Op: op, IsDir: isDir, Reconcile: isDir, At: time.Now()}:
	default:
		w.dirty = true
	}
	return nil
}

func (w *FSEvents) loop(ctx context.Context) {
	defer close(w.done)
	defer close(w.events)
	defer close(w.errors)
	defer func() { C.fw_events_close(w.stream); w.handle.Delete() }()
	health := time.NewTicker(time.Second)
	defer health.Stop()
	for {
		var delivery chan RawEvent
		if w.dirty {
			delivery = w.events
		}
		select {
		case <-ctx.Done():
			return
		case <-health.C:
			if err := w.checkRoot(); err != nil {
				w.warn(err)
				return
			}
		case <-w.lost:
			w.dirty = true
		case delivery <- RawEvent{Path: ".", Reconcile: true, At: time.Now()}:
			w.dirty = false
		case call := <-w.calls:
			err := call.ctx.Err()
			if err == nil {
				err = w.checkRoot()
			}
			if err == nil {
				err = pruneScopes(call.ctx, w.opts.Filter)
			}
			call.result <- err
		case e := <-w.incoming:
			if err := w.translate(e); err != nil {
				w.warn(err)
				if err == ErrRootGone {
					return
				}
			}
		}
	}
}

func (w *FSEvents) Reconcile(ctx context.Context) error {
	w.mu.Lock()
	started, closed, calls, done := w.started, w.closed, w.calls, w.done
	w.mu.Unlock()
	if !started || closed {
		return ErrClosed
	}
	call := reconcileCall{ctx: ctx, result: make(chan error, 1)}
	select {
	case calls <- call:
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return ErrClosed
	}
	select {
	case err := <-call.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return ErrClosed
	}
}

func (w *FSEvents) Close() error {
	w.mu.Lock()
	w.closed = true
	cancel, done, started := w.cancel, w.done, w.started
	w.mu.Unlock()
	if started {
		cancel()
		<-done
	}
	return nil
}

var _ Watcher = (*FSEvents)(nil)
