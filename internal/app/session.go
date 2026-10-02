package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"os"
	"sync"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/debounce"
	"github.com/StevenWinsir/FolderWatch/internal/eventnorm"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

var ErrSessionClosed = errors.New("monitoring session closed")

// Event publishes semantic batches and baseline generations. Paths remains
// diagnostic compatibility metadata, never a verdict for UI state. On output
// overflow Batch.Reload requires ChangeState, not reconstruction from Paths.
type Event struct {
	Batch         *changes.Batch      `json:"batch,omitempty"`
	Type          string              `json:"type"`
	Sequence      uint64              `json:"sequence"`
	Generation    uint64              `json:"generation"`
	Paths         []eventnorm.Request `json:"paths,omitempty"`
	Reconcile     bool                `json:"reconcile,omitempty"`
	BaselineFiles int                 `json:"baseline_files,omitempty"`
	Message       string              `json:"message,omitempty"`
}

type resetCall struct {
	ctx    context.Context
	result chan error
}

// Session is an application owner independent of UI and native watcher details.
// There is one command/event owner, one watcher and one coalescer, not a goroutine
// per path. Public event backpressure folds into a root reconciliation.
type Session struct {
	root                 string
	filter               ignore.Filter
	ctx                  context.Context
	cancel               context.CancelFunc
	watcher              watcher.Watcher
	snapshots            *snapshot.Store
	changes              *changes.Store
	batches              <-chan debounce.Batch
	watchErrors          <-chan error
	events               chan Event
	commands             chan resetCall
	done                 chan struct{}
	mu                   sync.Mutex
	err                  error
	sequence, generation uint64
}

func StartSession(parent context.Context, prepared Prepared) (*Session, error) {
	cfg := prepared.Config
	if err := cfg.Validate(); err != nil {
		return nil, &InputError{Err: err}
	}
	if prepared.Matcher == nil {
		return nil, &InputError{Err: fmt.Errorf("missing prepared ignore matcher")}
	}
	if prepared.Matcher.Root() != cfg.Root {
		return nil, &InputError{Err: fmt.Errorf("prepared config and matcher roots differ")}
	}
	ctx, cancel := context.WithCancel(parent)
	snapshotOpts := snapshot.Options{MaxFileBytes: cfg.MaxSnapshotBytes, MemoryFileBytes: 64 << 10, MemoryBytes: cfg.SnapshotMemoryBytes, DiskBytes: cfg.SnapshotCacheBytes, MaxFiles: cfg.MaxSnapshotFiles}
	store, err := snapshot.New(cfg.Root, snapshotOpts)
	if err != nil {
		cancel()
		return nil, err
	}
	adapter, err := watcher.New(watcher.Options{Filter: prepared.Matcher, EventBuffer: cfg.MaxPendingEvents, MaxDirectories: cfg.MaxWatchDirs})
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	raw, watchErrors, err := adapter.Start(ctx, cfg.Root)
	if err != nil {
		cancel()
		_ = store.Close()
		return nil, err
	}
	batches, err := debounce.Start(ctx, cfg.Root, prepared.Matcher, raw, debounce.Options{Delay: cfg.Debounce, MaxPending: cfg.MaxPendingEvents})
	if err != nil {
		cancel()
		_ = adapter.Close()
		_ = store.Close()
		return nil, err
	}
	s := &Session{root: cfg.Root, filter: prepared.Matcher, ctx: ctx, cancel: cancel, watcher: adapter, snapshots: store, batches: batches, watchErrors: watchErrors, events: make(chan Event, 32), commands: make(chan resetCall), done: make(chan struct{})}
	// A fresh scan happens AFTER watcher registration, never trusting the older
	// Prepared inventory as a content baseline. Startup-window events remain queued.
	if err := s.captureBaseline(ctx); err != nil {
		cancel()
		_ = adapter.Close()
		for range batches {
		}
		_ = store.Close()
		return nil, err
	}
	diffOpts := diff.Defaults()
	diffOpts.MaxBytes = cfg.MaxDiffBytes
	diffOpts.MaxLines = cfg.MaxDiffLines
	s.changes, err = changes.New(cfg.Root, prepared.Matcher, store, changes.Options{Snapshot: snapshotOpts, Diff: diffOpts, MaxEntries: cfg.MaxSnapshotFiles})
	if err != nil {
		cancel()
		_ = adapter.Close()
		for range batches {
		}
		_ = store.Close()
		return nil, err
	}
	baseline := store.Baseline()
	s.generation = baseline.Generation
	s.publish(Event{Type: "ready", Reconcile: true, BaselineFiles: len(baseline.Files)})
	go s.loop()
	return s, nil
}

func (s *Session) captureBaseline(ctx context.Context) error {
	if err := s.watcher.Reconcile(ctx); err != nil {
		return err
	}
	result, err := scan.Scan(ctx, s.root, s.filter)
	if err != nil {
		return err
	}
	// Do not silently erase previously captured paths when permissions fail.
	// --scan remains best-effort; a baseline is intentionally all-or-nothing.
	if len(result.Warnings) > 0 {
		return fmt.Errorf("baseline requires a complete scan: %d warning(s); first at %q: %s", len(result.Warnings), result.Warnings[0].Path, result.Warnings[0].Message)
	}
	paths := make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		paths = append(paths, entry.Path)
	}
	if s.changes != nil {
		_, err := s.changes.Reset(ctx, paths)
		return err
	}
	return s.snapshots.Reset(ctx, paths)
}

func (s *Session) publish(e Event) {
	if e.Batch == nil && s.changes != nil && (e.Type == "ready" || e.Type == "reset") {
		g, v := s.changes.Head()
		e.Batch = &changes.Batch{Generation: g, Version: v, Reload: true}
	}
	s.sequence++
	e.Sequence = s.sequence
	e.Generation = s.generation
	select {
	case s.events <- e:
		return
	default:
	}
	for {
		select {
		case <-s.events:
		default:
			goto drained
		}
	}
drained:
	e.Reconcile = true
	e.Paths = nil
	if s.changes != nil {
		generation, version := s.changes.Head()
		e.Batch = &changes.Batch{Generation: generation, Version: version, Reload: true}
	}
	if e.Type == "paths" {
		e.Type = "reconcile"
	}
	s.events <- e // only this owner sends; draining guarantees a slot
}

func (s *Session) fail(err error) { s.mu.Lock(); s.err = errors.Join(s.err, err); s.mu.Unlock() }

func (s *Session) loop() {
	defer close(s.done)
	defer close(s.events)
	defer func() {
		s.cancel()
		if err := s.watcher.Close(); err != nil {
			s.fail(err)
		}
		for range s.batches {
		} // wait for the coalescer's timer/owner to terminate
		if s.changes != nil {
			if err := s.changes.Close(); err != nil {
				s.fail(err)
			}
		}
		if err := s.snapshots.Close(); err != nil {
			s.fail(err)
		}
	}()
	errorsIn := s.watchErrors
	// Retry only failed/transient reconciliation, with bounded backoff. This is
	// also the post-capture reconciliation closing the startup observation window.
	retry := time.NewTimer(0)
	defer retry.Stop()
	retryC := retry.C
	retryDelay := 100 * time.Millisecond
	schedule := func() {
		if retryC == nil {
			retry.Reset(retryDelay)
			retryC = retry.C
			if retryDelay < 2*time.Second {
				retryDelay *= 2
			}
			if retryDelay > 2*time.Second {
				retryDelay = 2 * time.Second
			}
		}
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-retryC:
			retryC = nil
			again, err := s.resolveChanges(debounce.Batch{Reconcile: true})
			if err != nil {
				if s.ctx.Err() == nil {
					s.fail(err)
				}
				return
			}
			if again {
				schedule()
			} else {
				retryDelay = 100 * time.Millisecond
			}
		case call := <-s.commands:
			ctx, cancel := context.WithCancel(call.ctx)
			stop := context.AfterFunc(s.ctx, cancel)
			err := s.captureBaseline(ctx)
			stop()
			cancel()
			if err == nil {
				baseline := s.snapshots.Baseline()
				s.generation = baseline.Generation
				// Old-generation publications cannot arrive after the reset marker. Queued
				// filesystem/coalescer work stays valid as new-generation invalidations.
				for {
					select {
					case <-s.events:
					default:
						goto resetDrained
					}
				}
			resetDrained:
				s.publish(Event{Type: "reset", Reconcile: true, BaselineFiles: len(baseline.Files)})
				schedule()
			}
			call.result <- err
		case batch, ok := <-s.batches:
			if !ok {
				if s.ctx.Err() == nil {
					s.fail(fmt.Errorf("watch pipeline ended unexpectedly"))
				}
				return
			}
			again, err := s.resolveChanges(batch)
			if err != nil {
				if s.ctx.Err() == nil {
					s.fail(err)
				}
				return
			}
			if again {
				schedule()
			}
		case err, ok := <-errorsIn:
			if !ok {
				errorsIn = nil
				continue
			}
			if errors.Is(err, watcher.ErrRootGone) || errors.Is(err, watcher.ErrDirectoryLimit) {
				s.fail(err)
				return
			}
			s.publish(Event{Type: "warning", Message: err.Error(), Reconcile: true})
			schedule()
		}
	}
}

func (s *Session) Changes() []changes.Summary { return s.changes.View().Changes }
func (s *Session) ChangeState() changes.View  { return s.changes.View() }
func (s *Session) GetDiff(ctx context.Context, path string) (diff.Result, error) {
	combined, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	return s.changes.GetDiff(combined, path)
}

func (s *Session) Events() <-chan Event          { return s.events }
func (s *Session) Done() <-chan struct{}         { return s.done }
func (s *Session) Baseline() snapshot.Baseline   { return s.snapshots.Baseline() }
func (s *Session) SnapshotStats() snapshot.Stats { return s.snapshots.Stats() }
func (s *Session) Err() error                    { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

func (s *Session) ReadBaseline(ctx context.Context, path string) ([]byte, error) {
	key, err := pathutil.Key(s.root, path)
	if err != nil {
		return nil, err
	}
	ref, ok := s.snapshots.Baseline().Files[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	combined, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	return s.snapshots.ReadContent(combined, ref)
}

// ResetBaseline is serialized with event publication. After command admission,
// await its definitive result even if the caller cancels; a committed reset is
// returned as success, never as an ambiguous timeout that invites blind retry.
func (s *Session) ResetBaseline(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := resetCall{ctx: ctx, result: make(chan error, 1)}
	select {
	case s.commands <- call:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrSessionClosed
	}
	select {
	case err := <-call.result:
		return err
	case <-s.done:
		select {
		case err := <-call.result:
			return err
		default:
			return ErrSessionClosed
		}
	}
}

func (s *Session) Close() error { s.cancel(); <-s.done; return s.Err() }
