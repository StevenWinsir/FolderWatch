package changes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/StevenWinsir/FolderWatch/internal/catalog"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

// Store owns semantics. Metadata is disk indexed; mutation and diff have
// separate cancellable tokens. Readers never observe an unfinished staged scan.
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
	generation     uint64
	items          *catalog.Store[Summary]
	version        uint64
	closed         bool
	closeErr       error
}

func New(root string, filter ignore.Filter, baseline *snapshot.Store, opts Options) (*Store, error) {
	if baseline == nil || filter == nil || opts.MaxEntries < 0 {
		return nil, fmt.Errorf("change store requires baseline, filter and non-negative entry cap")
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
	generation, _ := baseline.Head()
	if generation == 0 {
		return nil, fmt.Errorf("baseline has not been captured")
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
	items, err := catalog.New[Summary](resolve.CacheDir())
	if err != nil {
		_ = resolve.Close()
		_ = scratch.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{root: root, filter: filter, snapshots: baseline, resolveScratch: resolve, diffScratch: scratch, opts: opts, engine: diff.BoundedLCS{}, ctx: ctx, cancel: cancel, op: make(chan struct{}, 1), diffOp: make(chan struct{}, 1), generation: generation, items: items}
	s.op <- struct{}{}
	s.diffOp <- struct{}{}
	if err := baseline.WalkUnknown(ctx, func(path, _ string) error { return items.Put(path, Summary{Path: path, Kind: Unknown, Version: 1}) }); err != nil {
		_ = s.Close()
		return nil, err
	}
	if items.Len() > 0 {
		s.version = 1
	}
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
func (s *Store) Head() (uint64, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation, s.version
}
func (s *Store) Lookup(path string) (Summary, uint64, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Summary{}, s.generation, false, ErrClosed
	}
	value, ok, err := s.items.Get(path)
	return value, s.generation, ok, err
}

// Page performs an ordered index walk without copying/sorting all summaries.
// Filter applies to the complete set, not just a page. Memory is O(page size).
// Expected generation/version fence continuation requests to one semantic view.
func (s *Store) Page(ctx context.Context, offset, limit int, query string, expectedGeneration, expectedVersion uint64) (Page, error) {
	if offset < 0 || limit < 1 || limit > 500 {
		return Page{}, ErrPage
	}
	ctx, done := s.linked(ctx)
	defer done()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Page{}, ErrClosed
	}
	if expectedGeneration != 0 && (expectedGeneration != s.generation || expectedVersion != s.version) {
		return Page{}, ErrStale
	}
	out := Page{Generation: s.generation, Version: s.version, Total: s.items.Len(), NextOffset: -1, Changes: []Summary{}}
	query = strings.ToLower(strings.TrimSpace(query))
	var match func(string) bool
	if query != "" {
		match = func(key string) bool { return strings.Contains(strings.ToLower(key), query) }
	}
	entries, matched, err := s.items.Window(ctx, offset, limit, match)
	for _, entry := range entries {
		out.Changes = append(out.Changes, entry.Value)
	}
	if err != nil {
		return Page{}, err
	}
	out.Matched = matched
	if offset > matched {
		return Page{}, ErrPage
	}
	if offset+len(out.Changes) < matched {
		out.NextOffset = offset + len(out.Changes)
	}
	return out, nil
}

// View is the explicit materializing compatibility API for CLI consumers.
// GUI/status/diff use Page/Head/Lookup instead. A storage failure is visible.
func (s *Store) View() View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := View{Generation: s.generation, Version: s.version, Changes: []Summary{}}
	if s.closed {
		return out
	}
	if err := s.items.Walk(s.ctx, "", func(_ string, item Summary) error { out.Changes = append(out.Changes, item); return nil }); err != nil {
		out.Error = err.Error()
	}
	return out
}

func (s *Store) Reset(ctx context.Context, files []string) (Batch, error) {
	return s.reset(ctx, func(work context.Context) error { return s.snapshots.Reset(work, files) })
}
func (s *Store) ResetFrom(ctx context.Context, source snapshot.Source) (Batch, error) {
	return s.reset(ctx, func(work context.Context) error { return s.snapshots.ResetFrom(work, source, false) })
}
func (s *Store) reset(ctx context.Context, capture func(context.Context) error) (Batch, error) {
	ctx, done := s.linked(ctx)
	defer done()
	if err := s.acquire(ctx, s.op); err != nil {
		return Batch{}, err
	}
	defer func() { s.op <- struct{}{} }()
	next, err := catalog.New[Summary](s.resolveScratch.CacheDir())
	if err != nil {
		return Batch{}, err
	}
	if err := capture(ctx); err != nil {
		_ = next.Close()
		return Batch{}, err
	}
	generation, _ := s.snapshots.Head()
	s.mu.Lock()
	old := s.items
	s.items = next
	s.generation = generation
	s.version++
	batch := Batch{Generation: s.generation, Version: s.version, Reload: true}
	s.mu.Unlock()
	if err := old.Close(); err != nil {
		s.mu.Lock()
		s.closeErr = errors.Join(s.closeErr, err)
		s.mu.Unlock()
	}
	return batch, nil
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
	s.closeErr = errors.Join(s.closeErr, s.items.Close(), s.resolveScratch.Close(), s.diffScratch.Close())
	return s.closeErr
}
