package watcher

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

// Adaptive keeps native resource budgets separate from the monitored tree's
// size. A failed native backend is closed completely before polling takes over;
// a root invalidation bridges the handover, including already-existing files.
// The low-level New/FSNotify API remains available for native regression tests.
type Adaptive struct {
	opts            Options
	factory         func(Options) (Watcher, error)
	mu              sync.Mutex
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	calls           chan reconcileCall
	events          chan RawEvent
	errors          chan error
	root            string
	identity        fs.FileInfo
	source          Watcher // owned by loop after Start
	in              <-chan RawEvent
	errs            <-chan error
	polling         bool
	dirty           bool
	closeErr        error
}

func NewAdaptive(opts Options) (*Adaptive, error) {
	if _, err := New(opts); err != nil {
		return nil, err
	}
	return &Adaptive{opts: opts, factory: preferredWatcher}, nil
}

func (w *Adaptive) checkRoot() error {
	info, err := os.Lstat(w.root)
	if err != nil || !info.IsDir() || !os.SameFile(info, w.identity) {
		return ErrRootGone
	}
	return nil
}

func (w *Adaptive) Start(parent context.Context, root string) (<-chan RawEvent, <-chan error, error) {
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
	w.source, err = w.factory(w.opts)
	if err == nil {
		w.in, w.errs, err = w.source.Start(ctx, w.root)
	}
	if err != nil {
		if ctx.Err() != nil {
			cancel()
			if w.source != nil {
				_ = w.source.Close()
			}
			return nil, nil, ctx.Err()
		}
		if err = w.fallback(ctx, err); err != nil {
			cancel()
			return nil, nil, err
		}
	}
	w.started = true
	go w.loop(ctx)
	return w.events, w.errors, nil
}

func (w *Adaptive) warn(err error) {
	select {
	case w.errors <- err:
	default:
		w.dirty = true
	}
}

func (w *Adaptive) fallback(ctx context.Context, reason error) error {
	if w.source != nil {
		if err := w.source.Close(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.checkRoot(); err != nil {
		return err
	}
	p := newPolling(w.opts)
	in, errs, err := p.Start(ctx, w.root)
	if err != nil {
		_ = p.Close()
		return fmt.Errorf("start polling fallback: %w", err)
	}
	if err := w.checkRoot(); err != nil {
		_ = p.Close()
		return err
	}
	w.source, w.in, w.errs, w.polling = p, in, errs, true
	w.dirty = true
	// Do not wrap ErrDirectoryLimit: this is a recovered resource condition,
	// not a fatal session error. The user is told about the latency tradeoff.
	w.warn(fmt.Errorf("native watching unavailable (%v); monitoring continues using periodic metadata scans (at least 2s between scans; large trees take longer)", reason))
	return nil
}

func nativeResourceError(err error) bool {
	return errors.Is(err, ErrDirectoryLimit) || errors.Is(err, ErrClosed) ||
		errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) || errors.Is(err, syscall.ENOSPC)
}

func (w *Adaptive) loop(ctx context.Context) {
	defer close(w.done)
	defer close(w.events)
	defer close(w.errors)
	defer func() { w.closeErr = w.source.Close() }()
	for {
		var delivery chan RawEvent
		if w.dirty {
			delivery = w.events
		}
		select {
		case <-ctx.Done():
			return
		case delivery <- RawEvent{Path: ".", Reconcile: true, At: time.Now()}:
			w.dirty = false
		case call := <-w.calls:
			combined, cancel := context.WithCancel(call.ctx)
			stop := context.AfterFunc(ctx, cancel)
			err := w.source.Reconcile(combined)
			if err != nil && combined.Err() == nil && !w.polling && nativeResourceError(err) {
				// The session context, not the command's short deadline, owns the
				// replacement backend. A cancelled command must not kill watching.
				err = w.fallback(ctx, err)
			}
			stop()
			cancel()
			call.result <- err
			if errors.Is(err, ErrRootGone) {
				w.warn(err)
				return
			}
		case e, ok := <-w.in:
			if !ok {
				if ctx.Err() != nil {
					return
				}
				if w.polling {
					w.warn(ErrClosed)
					return
				}
				if err := w.fallback(ctx, ErrClosed); err != nil {
					w.warn(err)
					return
				}
				continue
			}
			if w.dirty {
				continue
			}
			select {
			case w.events <- e:
			default:
				w.dirty = true
			}
		case err, ok := <-w.errs:
			if !ok {
				w.errs = nil
				continue
			}
			if errors.Is(err, ErrRootGone) {
				w.warn(err)
				return
			}
			if !w.polling && nativeResourceError(err) {
				if err := w.fallback(ctx, err); err != nil {
					w.warn(err)
					return
				}
			} else {
				w.warn(err)
			}
		}
	}
}

func (w *Adaptive) Reconcile(ctx context.Context) error {
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

func (w *Adaptive) Close() error {
	w.mu.Lock()
	w.closed = true
	cancel, done, started := w.cancel, w.done, w.started
	w.mu.Unlock()
	if started {
		cancel()
		<-done
		return w.closeErr
	}
	return nil
}

var _ Watcher = (*Adaptive)(nil)
