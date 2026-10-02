package changes

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

// Store serializes resolving/reset through a cancellable ownership token.
// List uses a short read lock; diff has its OWN scratch reader and concurrency
// token, so expensive diff work cannot block the watcher/resolve owner.
// The caller owns the supplied baseline Store. Close releases only scratch data.
type Store struct {
	root           string
	filter         ignore.Filter
	snapshots      *snapshot.Store
	resolveScratch *snapshot.Store
	diffScratch    *snapshot.Store
	opts           Options
	engine         diff.Engine
	ctx            context.Context
	cancel         context.CancelFunc
	op             chan struct{}
	diffOp         chan struct{}
	mu             sync.RWMutex
	baseline       snapshot.Baseline
	items          map[string]Summary
	version        uint64
	closed         bool
	closeErr       error
}

func New(root string, filter ignore.Filter, baseline *snapshot.Store, opts Options) (*Store, error) {
	if baseline == nil || filter == nil || opts.MaxEntries < 1 {
		return nil, fmt.Errorf("change store requires baseline, filter and positive entry cap")
	}
	if err := opts.Diff.Validate(); err != nil {
		return nil, err
	}
	root, err := pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return nil, err
	}
	if baseline.Root() != root {
		return nil, fmt.Errorf("baseline belongs to a different root")
	}
	if rooted, ok := filter.(interface{ Root() string }); ok && rooted.Root() != root {
		return nil, fmt.Errorf("ignore matcher belongs to a different root")
	}
	captureOpts := opts.Snapshot
	captureOpts.MaxFiles = 1
	captureOpts.MemoryBytes = 0
	captureOpts.DiskBytes = 0
	resolve, err := snapshot.New(root, captureOpts)
	if err != nil {
		return nil, err
	}
	diffOpts := opts.Snapshot
	diffOpts.MaxFiles = 1
	scratch, err := snapshot.New(root, diffOpts)
	if err != nil {
		_ = resolve.Close()
		return nil, err
	}
	b := baseline.Baseline()
	if b.Generation == 0 {
		_ = resolve.Close()
		_ = scratch.Close()
		return nil, fmt.Errorf("baseline has not been captured")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{root: root, filter: filter, snapshots: baseline, resolveScratch: resolve, diffScratch: scratch, opts: opts, engine: diff.BoundedLCS{}, ctx: ctx, cancel: cancel, op: make(chan struct{}, 1), diffOp: make(chan struct{}, 1), baseline: b, items: make(map[string]Summary)}
	s.op <- struct{}{}
	s.diffOp <- struct{}{}
	return s, nil
}

func (s *Store) linked(ctx context.Context) (context.Context, func()) {
	combined, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	return combined, func() { stop(); cancel() }
}
func (s *Store) acquire(ctx context.Context, token chan struct{}) error {
	if s.ctx.Err() != nil {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return ErrClosed
	case <-token:
	}
	if s.ctx.Err() != nil {
		token <- struct{}{}
		return ErrClosed
	}
	return nil
}

func (s *Store) Head() (generation, version uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baseline.Generation, s.version
}

func (s *Store) View() View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := View{Generation: s.baseline.Generation, Version: s.version, Changes: make([]Summary, 0, len(s.items))}
	for _, item := range s.items {
		v.Changes = append(v.Changes, copySummary(item))
	}
	sort.Slice(v.Changes, func(i, j int) bool { return v.Changes[i].Path < v.Changes[j].Path })
	return v
}

// Reset publishes a new baseline and clears the semantic list only after a
// complete successful capture. Failed/cancelled captures preserve both stores.
func (s *Store) Reset(ctx context.Context, files []string) (Batch, error) {
	ctx, done := s.linked(ctx)
	defer done()
	if err := s.acquire(ctx, s.op); err != nil {
		return Batch{}, err
	}
	defer func() { s.op <- struct{}{} }()
	if err := s.snapshots.Reset(ctx, files); err != nil {
		return Batch{}, err
	}
	b := s.snapshots.Baseline()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseline = b
	s.items = make(map[string]Summary)
	s.version++
	return Batch{Generation: b.Generation, Version: s.version, Reload: true}, nil
}

func (s *Store) Close() error {
	s.cancel()
	<-s.op
	defer func() { s.op <- struct{}{} }()
	<-s.diffOp
	defer func() { s.diffOp <- struct{}{} }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	s.closeErr = errors.Join(s.resolveScratch.Close(), s.diffScratch.Close())
	s.items = nil
	s.baseline.Files = nil
	return s.closeErr
}
