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

	"github.com/StevenWinsir/FolderWatch/internal/catalog"
	"github.com/StevenWinsir/FolderWatch/internal/filemeta"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
)

type fingerprint struct {
	Signature filemeta.Signature
	Kind      model.EntryKind
}

func sameFingerprint(a, b fingerprint) bool {
	return a.Kind == b.Kind && a.Signature.Same(b.Signature)
}

// polling is a resource fallback, not another baseline or change resolver. It
// reads metadata only and emits invalidations into the existing core pipeline.
// Metadata uses a fixed number of catalog descriptors, never one per file.
// There are no recursive goroutines or overlapping scans.
type polling struct {
	opts            Options
	interval        time.Duration
	mu              sync.Mutex
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	calls           chan reconcileCall
	events          chan RawEvent
	errors          chan error
	root            string
	policy          filemeta.Policy
	identity        fs.FileInfo
	previous        *catalog.Store[fingerprint]
	cacheDir        string
	closeErr        error
	dirty           bool
}

func newPolling(opts Options) *polling { return &polling{opts: opts, interval: 2 * time.Second} }

func (p *polling) checkRoot() error {
	info, err := os.Lstat(p.root)
	if err != nil || !info.IsDir() || !os.SameFile(info, p.identity) {
		return ErrRootGone
	}
	return nil
}

func (p *polling) Start(parent context.Context, root string) (<-chan RawEvent, <-chan error, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, ErrClosed
	}
	if p.started {
		return nil, nil, fmt.Errorf("watcher already started")
	}
	if err := parent.Err(); err != nil {
		return nil, nil, err
	}
	var err error
	p.root, err = pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return nil, nil, err
	}
	p.identity, err = os.Lstat(p.root)
	if err != nil {
		return nil, nil, err
	}
	p.policy = filemeta.PolicyForRoot(p.root)
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.events = make(chan RawEvent, p.opts.EventBuffer)
	p.errors = make(chan error, 16)
	p.calls = make(chan reconcileCall)
	p.done = make(chan struct{})
	temp, err := pathutil.NormalizeRoot(os.TempDir(), ".")
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if _, err := pathutil.Key(p.root, temp); err == nil {
		cancel()
		return nil, nil, fmt.Errorf("polling cache must be outside watched root")
	}
	p.cacheDir, err = os.MkdirTemp(temp, "folderwatch-poll-")
	if err == nil {
		p.previous, err = catalog.New[fingerprint](p.cacheDir)
	}
	if err == nil {
		err = p.sweep(ctx, true)
	}
	if err != nil {
		cancel()
		if p.previous != nil {
			_ = p.previous.Close()
		}
		if p.cacheDir != "" {
			_ = os.RemoveAll(p.cacheDir)
		}
		return nil, nil, err
	}
	p.started = true
	go p.loop(ctx)
	return p.events, p.errors, nil
}

func pruneScopes(ctx context.Context, filter interface{}) error {
	if cache, ok := filter.(interface{ PruneDirectories(context.Context) error }); ok {
		return cache.PruneDirectories(ctx)
	}
	return ctx.Err()
}

// scan stages metadata and protects failed scopes individually. A busy or
// unreadable descendant never discards healthy siblings or cancels failover.
func (p *polling) scan(ctx context.Context) (*catalog.Store[fingerprint], error) {
	if err := p.checkRoot(); err != nil {
		return nil, err
	}
	if err := pruneScopes(ctx, p.opts.Filter); err != nil {
		return nil, err
	}
	next, err := catalog.New[fingerprint](p.cacheDir)
	if err != nil {
		return nil, err
	}
	blocked, err := catalog.New[bool](p.cacheDir)
	if err != nil {
		_ = next.Close()
		return nil, err
	}
	defer blocked.Close()
	var first error
	warning := func(path string, err error) error {
		if first == nil {
			first = fmt.Errorf("poll metadata %q: %w", path, err)
		}
		return blocked.Put(path, true)
	}
	pending := make([]catalog.Entry[fingerprint], 0, catalog.PageSize)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		err := next.Apply(pending)
		pending = pending[:0]
		return err
	}
	err = scan.Stream(ctx, p.root, ".", p.cacheDir, p.opts.Filter, func(meta model.FileMeta, walkErr error) error {
		if walkErr != nil {
			return warning(meta.Path, walkErr)
		}
		info, err := os.Lstat(filepath.Join(p.root, filepath.FromSlash(meta.Path)))
		if err != nil {
			return warning(meta.Path, err)
		}
		pending = append(pending, catalog.Entry[fingerprint]{Key: meta.Path, Value: fingerprint{Signature: p.policy.Read(info), Kind: meta.Kind}})
		if len(pending) == catalog.PageSize {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	}
	if err == nil && blocked.Len() > 0 {
		err = p.previous.Walk(ctx, "", func(key string, old fingerprint) error {
			for scope := key; ; {
				_, protected, err := blocked.Get(scope)
				if err != nil {
					return err
				}
				if protected {
					return next.Put(key, old)
				}
				if scope == "." {
					break
				}
				if at := strings.LastIndexByte(scope, '/'); at >= 0 {
					scope = scope[:at]
				} else {
					scope = "."
				}
			}
			return nil
		})
	}
	if err == nil {
		err = p.checkRoot()
	}
	if err != nil {
		_ = next.Close()
		return nil, err
	}
	if first != nil {
		select {
		case p.errors <- first:
		default:
		}
		p.dirty = true
	}
	return next, nil
}

func (p *polling) emit(e RawEvent) {
	if p.dirty {
		return
	}
	select {
	case p.events <- e:
	default:
		p.dirty = true
	}
}

func (p *polling) sweep(ctx context.Context, initial bool) error {
	next, err := p.scan(ctx)
	if err != nil {
		p.dirty = true
		return err
	}
	if !initial {
		err = next.Walk(ctx, "", func(key string, current fingerprint) error {
			if key == "." {
				return nil
			}
			old, exists, err := p.previous.Get(key)
			if err != nil {
				return err
			}
			if exists && sameFingerprint(old, current) {
				return nil
			}
			isDir := current.Kind == model.Directory || exists && old.Kind == model.Directory
			if current.Kind == model.Directory && exists && old.Kind == model.Directory && old.Signature.Device == current.Signature.Device && old.Signature.Inode == current.Signature.Inode && old.Signature.Mode == current.Signature.Mode {
				return nil
			}
			op := Write
			if !exists {
				op = Create
			}
			p.emit(RawEvent{Path: key, Op: op, IsDir: isDir, Reconcile: isDir, At: time.Now()})
			return nil
		})
		if err == nil {
			err = p.previous.Walk(ctx, "", func(key string, old fingerprint) error {
				if _, exists, err := next.Get(key); exists || err != nil {
					return err
				}
				isDir := old.Kind == model.Directory
				p.emit(RawEvent{Path: key, Op: Remove, IsDir: isDir, Reconcile: isDir, At: time.Now()})
				return nil
			})
		}
	}
	if err != nil {
		_ = next.Close()
		p.dirty = true
		return err
	}
	old := p.previous
	p.previous = next
	return old.Close()
}

func (p *polling) loop(ctx context.Context) {
	defer close(p.done)
	defer close(p.events)
	defer close(p.errors)
	defer func() {
		p.closeErr = errors.Join(p.previous.Close(), os.RemoveAll(p.cacheDir))
		p.previous = nil
	}()
	timer := time.NewTimer(p.interval)
	defer timer.Stop()
	health := time.NewTicker(time.Second)
	defer health.Stop()
	for {
		var delivery chan RawEvent
		if p.dirty {
			delivery = p.events
		}
		select {
		case <-ctx.Done():
			return
		case <-health.C:
			if err := p.checkRoot(); err != nil {
				select {
				case p.errors <- err:
				default:
				}
				return
			}
		case delivery <- RawEvent{Path: ".", Reconcile: true, At: time.Now()}:
			p.dirty = false
		case call := <-p.calls:
			// Reconcile is a coverage/identity check. Core performs the full
			// authoritative hash pass; a second metadata sweep here would double
			// I/O and cause feedback loops when the root is already dirty.
			err := call.ctx.Err()
			if err == nil {
				err = p.checkRoot()
			}
			call.result <- err
		case <-timer.C:
			start := time.Now()
			err := p.sweep(ctx, false)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case p.errors <- err:
				default:
				}
				if err == ErrRootGone {
					return
				}
			}
			// Wait AFTER finishing. Large scans cannot form a permanently full
			// work queue; duty cycle for polling is at most roughly 1/6.
			delay := time.Since(start) * 5
			if delay < p.interval {
				delay = p.interval
			}
			timer.Reset(delay)
		}
	}
}

func (p *polling) Reconcile(ctx context.Context) error {
	p.mu.Lock()
	started, closed, calls, done := p.started, p.closed, p.calls, p.done
	p.mu.Unlock()
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

func (p *polling) Close() error {
	p.mu.Lock()
	p.closed = true
	cancel, done, started := p.cancel, p.done, p.started
	p.mu.Unlock()
	if started {
		cancel()
		<-done
		return p.closeErr
	}
	return nil
}

var _ Watcher = (*polling)(nil)
