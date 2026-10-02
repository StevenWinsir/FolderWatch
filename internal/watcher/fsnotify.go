package watcher

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/fsnotify/fsnotify"
)

type reconcileCall struct {
	ctx    context.Context
	result chan error
}

// FSNotify owns one event loop. Directory bookkeeping never leaves that owner.
// Start/Close are serialized; Close is safe and idempotent even before Start.
type FSNotify struct {
	opts     Options
	mu       sync.Mutex
	started  bool
	closed   bool
	root     string
	identity fs.FileInfo
	native   *fsnotify.Watcher
	cancel   context.CancelFunc
	done     chan struct{}
	events   chan RawEvent
	errors   chan error
	calls    chan reconcileCall
	dirs     map[string]fs.FileInfo
	dirty    bool
}

func New(opts Options) (*FSNotify, error) {
	if opts.Filter == nil || opts.EventBuffer < 1 || opts.EventBuffer > 1<<20 || opts.MaxDirectories < 1 {
		return nil, fmt.Errorf("watcher needs a filter, 1..1048576 event slots and a positive directory limit")
	}
	return &FSNotify{opts: opts}, nil
}

func (w *FSNotify) Start(parent context.Context, root string) (<-chan RawEvent, <-chan error, error) {
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
	root, err := pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return nil, nil, err
	}
	identity, err := os.Lstat(root)
	if err != nil {
		return nil, nil, err
	}
	native, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	w.root, w.identity, w.native, w.cancel = root, identity, native, cancel
	w.events = make(chan RawEvent, w.opts.EventBuffer)
	w.errors = make(chan error, 16)
	w.calls = make(chan reconcileCall)
	w.done = make(chan struct{})
	w.dirs = make(map[string]fs.FileInfo)
	// Watch each parent before descending; startup changes remain queued by the OS.
	err = w.syncTree(ctx)
	if err != nil {
		cancel()
		_ = native.Close()
		return nil, nil, err
	}
	w.started = true
	go w.loop(ctx)
	return w.events, w.errors, nil
}

func (w *FSNotify) warn(err error) {
	select {
	case w.errors <- err:
	default:
		w.dirty = true
	}
}

func (w *FSNotify) emit(e RawEvent) {
	if w.dirty {
		return
	}
	select {
	case w.events <- e:
	default:
		w.dirty = true
		w.warn(ErrOverflow)
	}
}

func (w *FSNotify) checkRoot() error {
	info, err := os.Lstat(w.root)
	if err != nil || !info.IsDir() || !os.SameFile(info, w.identity) {
		return ErrRootGone
	}
	return nil
}

// syncTree refreshes registrations, not file contents. Unreadable descendants
// warn and remain retryable on the next reconciliation. Root failure is fatal.
func (w *FSNotify) syncTree(ctx context.Context) error {
	if err := w.checkRoot(); err != nil {
		return err
	}
	// The OS may automatically remove watches, or an overflow may hide a
	// delete/recreate at the same pathname. Compare identity and native state.
	active := make(map[string]bool)
	for _, p := range w.native.WatchList() {
		active[p] = true
	}
	for p, identity := range w.dirs {
		key, err := pathutil.Key(w.root, p)
		if err == nil {
			err = pathutil.CheckParents(w.root, key+"/_")
		}
		var info fs.FileInfo
		if err == nil {
			info, err = os.Lstat(p)
		}
		if !active[p] || err != nil || !info.IsDir() || !os.SameFile(identity, info) {
			w.forgetRemovedScope(p, identity)
			_ = w.native.Remove(p)
			delete(w.dirs, p)
		}
	}
	seen := make(map[string]bool)
	err := filepath.WalkDir(w.root, func(p string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if p == w.root {
				return walkErr
			}
			w.warn(fmt.Errorf("watch directory %q: %w", p, walkErr))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		ignored, err := w.opts.Filter.Match(p, true)
		if err != nil || ignored {
			if p == w.root && err != nil {
				return err
			}
			if err != nil {
				w.warn(err)
			}
			return fs.SkipDir
		}
		if w.dirs[p] == nil {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.IsDir() {
				w.dirty = true
				return fs.SkipDir
			}
			if len(w.dirs) >= w.opts.MaxDirectories {
				return fmt.Errorf("%w: %d", ErrDirectoryLimit, w.opts.MaxDirectories)
			}
			if err := w.native.Add(p); err != nil {
				return fmt.Errorf("register %q: %w", p, err)
			}
			w.dirs[p] = info
		}
		seen[p] = true
		return nil
	})
	if err != nil {
		return err
	}
	for p := range w.dirs {
		if !seen[p] {
			_ = w.native.Remove(p)
			delete(w.dirs, p)
		}
	}
	return nil
}

func (w *FSNotify) dropTree(p string) {
	for dir, identity := range w.dirs {
		if dir == p || strings.HasPrefix(dir, p+string(filepath.Separator)) {
			w.forgetRemovedScope(dir, identity)
			_ = w.native.Remove(dir)
			delete(w.dirs, dir)
		}
	}
}

func (w *FSNotify) forgetRemovedScope(p string, identity fs.FileInfo) {
	key, err := pathutil.Key(w.root, p)
	if err != nil {
		return
	}
	var info fs.FileInfo
	err = pathutil.CheckParents(w.root, key+"/_")
	if err == nil {
		info, err = os.Lstat(p)
	}
	if err == nil && info.IsDir() && os.SameFile(info, identity) {
		return
	}
	if cache, ok := w.opts.Filter.(interface{ ForgetDirectory(string) }); ok {
		cache.ForgetDirectory(key)
	}
}

func (w *FSNotify) handle(ctx context.Context, e fsnotify.Event) error {
	// kqueue can deliver an already-removed watch descriptor without a name.
	// Its scope is unknowable, so recover globally rather than emit an empty key.
	if e.Name == "" {
		w.dirty = true
		return nil
	}
	key, err := pathutil.Key(w.root, e.Name)
	if err != nil {
		return err
	}
	wasDir := w.dirs[e.Name] != nil
	if key == "." && e.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		return ErrRootGone
	}
	if key == "." {
		// Root notifications are invalidations, not proof that the path still
		// names the original directory. Native events can also be entirely
		// absent after unlink while Linux retains a cwd/open reference; the
		// event-loop health check below covers that independent case.
		if err := w.checkRoot(); err != nil {
			return err
		}
	}
	if wasDir && e.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.dropTree(e.Name)
	}
	info, statErr := os.Lstat(e.Name)
	isDir := wasDir || statErr == nil && info.IsDir()
	// For a replaced link use its actual kind, not stale directory metadata.
	if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		isDir = false
	}
	ignored, err := w.opts.Filter.Match(key, isDir)
	if err != nil {
		return err
	}
	if ignored {
		return nil
	}
	var op Op
	for native, local := range map[fsnotify.Op]Op{fsnotify.Create: Create, fsnotify.Write: Write, fsnotify.Remove: Remove, fsnotify.Rename: Rename, fsnotify.Chmod: Chmod} {
		if e.Op&native != 0 {
			op |= local
		}
	}
	if isDir && e.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
		if err := w.syncTree(ctx); err != nil {
			return err
		}
		// Includes files created/moved into a directory before registration completed.
		w.emit(RawEvent{Path: key, Op: op, IsDir: true, Reconcile: true, At: time.Now()})
	} else {
		w.emit(RawEvent{Path: key, Op: op, IsDir: isDir, At: time.Now()})
	}
	return nil
}

func (w *FSNotify) loop(ctx context.Context) {
	defer close(w.done)
	defer close(w.events)
	defer close(w.errors)
	defer w.native.Close()
	// One O(1) root-identity health check, never a recursive scan. On Linux,
	// unlinking a directory held as cwd/open fd can yield NO native event
	// until the last reference closes. This must remain live during Pause.
	// Avoid watching the parent: that can enroll unrelated files on kqueue.
	rootHealth := time.NewTicker(time.Second)
	defer rootHealth.Stop()
	for {
		var delivery chan RawEvent
		if w.dirty {
			delivery = w.events
		}
		select {
		case <-ctx.Done():
			return
		case <-rootHealth.C:
			if err := w.checkRoot(); err != nil {
				w.warn(err)
				return
			}
		case delivery <- RawEvent{Path: ".", Reconcile: true, At: time.Now()}:
			w.dirty = false
		case call := <-w.calls:
			combined, cancel := context.WithCancel(call.ctx)
			stop := context.AfterFunc(ctx, cancel)
			err := w.syncTree(combined)
			stop()
			cancel()
			call.result <- err
			if errors.Is(err, ErrRootGone) || errors.Is(err, ErrDirectoryLimit) {
				w.warn(err)
				return
			}
		case e, ok := <-w.native.Events:
			if !ok {
				w.warn(ErrClosed)
				return
			}
			if err := w.handle(ctx, e); err != nil {
				w.warn(err)
				w.dirty = true
				if errors.Is(err, ErrRootGone) || errors.Is(err, ErrDirectoryLimit) {
					return
				}
			}
		case err, ok := <-w.native.Errors:
			if !ok {
				w.warn(ErrClosed)
				return
			}
			w.nativeFailure(err)
		}
	}
}

// A native child may disappear between ReadDir and Info during a legitimate
// rename/delete. Treat that race as an explicit root invalidation, not a user
// warning. Other native errors remain visible; root loss has its own sentinel.
func (w *FSNotify) nativeFailure(err error) {
	w.dirty = true
	if !errors.Is(err, fs.ErrNotExist) {
		w.warn(err)
	}
}

func (w *FSNotify) Reconcile(ctx context.Context) error {
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

func (w *FSNotify) Close() error {
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

var _ Watcher = (*FSNotify)(nil)
