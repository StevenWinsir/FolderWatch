package snapshot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

type stored struct {
	ref     Ref
	content []byte
	file    string
}

// Store serializes mutation separately from publication. Reset builds an isolated
// generation, then swaps it once; readers never see half a new baseline.
// Each reset can temporarily use twice the configured content budgets.
type Store struct {
	root         string
	ctx          context.Context
	cancel       context.CancelFunc
	opts         Options
	op           chan struct{}
	mu           sync.RWMutex
	closed       bool
	closeErr     error
	dir          string
	retired      []string
	seq          uint64
	items        map[string]stored
	baseline     Baseline
	memory, disk int64
}

func New(root string, opts Options) (*Store, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	root, err := pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return nil, err
	}
	temp := opts.TempDir
	if temp == "" {
		temp = os.TempDir()
	}
	temp, err = pathutil.NormalizeRoot(temp, ".")
	if err != nil {
		return nil, fmt.Errorf("snapshot temp directory: %w", err)
	}
	if _, err := pathutil.Key(root, temp); err == nil {
		return nil, fmt.Errorf("snapshot cache must be outside monitored root")
	}
	dir, err := os.MkdirTemp(temp, "folderwatch-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	opts.TempDir = temp
	ctx, cancel := context.WithCancel(context.Background())
	s := &Store{root: root, ctx: ctx, cancel: cancel, opts: opts, op: make(chan struct{}, 1), dir: dir, items: make(map[string]stored), baseline: Baseline{Files: make(map[string]Ref)}}
	s.op <- struct{}{}
	return s, nil
}

func (s *Store) Capture(ctx context.Context, path string) (Ref, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if err := s.acquire(ctx); err != nil {
		return Ref{}, err
	}
	defer s.release()
	if s.closed {
		return Ref{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return Ref{}, err
	}
	s.mu.RLock()
	full := len(s.items) >= s.opts.MaxFiles
	s.mu.RUnlock()
	if full {
		return Ref{}, fmt.Errorf("snapshot reference limit %d exceeded", s.opts.MaxFiles)
	}
	var item stored
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		item, err = s.capture(ctx, path)
		if err != ErrUnstable {
			break
		}
	}
	if err != nil {
		return Ref{}, err
	}
	s.mu.Lock()
	s.items[item.ref.ID] = item
	if item.content != nil {
		s.memory += int64(len(item.content))
	}
	if item.file != "" {
		s.disk += item.ref.Meta.Size
	}
	s.mu.Unlock()
	return item.ref, nil
}

func (s *Store) Baseline() Baseline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Baseline{Generation: s.baseline.Generation, Files: make(map[string]Ref, len(s.baseline.Files))}
	for key, ref := range s.baseline.Files {
		out.Files[key] = ref
	}
	return out
}

func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Stats{References: len(s.items), MemoryBytes: s.memory, DiskBytes: s.disk}
}

func (s *Store) Reset(ctx context.Context, files []string) (resultErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return ErrClosed
	}
	if len(files) > s.opts.MaxFiles {
		return fmt.Errorf("baseline exceeds snapshot entry limit %d", s.opts.MaxFiles)
	}
	// A failed cleanup cannot accumulate another retired generation.
	for _, dir := range s.retired {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("clean retired snapshot: %w", err)
		}
	}
	s.retired = nil
	if err := ctx.Err(); err != nil {
		return err
	}
	stage, err := New(s.root, s.opts)
	if err != nil {
		return err
	}
	defer func() {
		if err := stage.Close(); err != nil {
			s.retire(stage.dir)
			resultErr = errors.Join(resultErr, fmt.Errorf("clean staging snapshot: %w", err))
		}
	}()
	next := make(map[string]Ref, len(files))
	for _, path := range files {
		key, err := pathutil.Key(s.root, path)
		if err != nil {
			return err
		}
		if _, exists := next[key]; exists {
			continue
		}
		ref, err := stage.Capture(ctx, key)
		if err != nil {
			return fmt.Errorf("capture baseline %q: %w", key, err)
		}
		next[key] = ref
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Preserve the previous complete generation until every requested capture succeeds.
	s.mu.Lock()
	// A reader may have delayed publication after the earlier cancellation check.
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	oldDir := s.dir
	s.dir, s.items, s.memory, s.disk, s.seq = stage.dir, stage.items, stage.memory, stage.disk, stage.seq
	s.baseline = Baseline{Generation: s.baseline.Generation + 1, Files: next}
	s.mu.Unlock()
	// Transfer ownership; stage.Close must not erase the just-published generation.
	stage.dir = ""
	stage.items = nil
	stage.memory = 0
	stage.disk = 0
	// Cleanup failure is retried before any next reset, preventing unbounded
	// retired generations; Close reports persistent cleanup errors.
	if err := os.RemoveAll(oldDir); err != nil {
		s.retire(oldDir)
	}
	return nil
}

// retire keeps only cache directories owned by this Store. Filesystem cleanup
// failures are surfaced by Close, never mistaken for a failed baseline commit.
func (s *Store) retire(dir string) { s.retired = append(s.retired, dir) }

func (s *Store) Delete(ref Ref) error {
	<-s.op
	defer s.release()
	if s.closed {
		return ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[ref.ID]
	if !ok || item.ref != ref {
		return ErrStale
	}
	for _, pinned := range s.baseline.Files {
		if pinned.ID == ref.ID {
			return fmt.Errorf("cannot delete a baseline-owned reference; reset the baseline")
		}
	}
	if item.file != "" {
		if err := os.Remove(item.file); err != nil && !os.IsNotExist(err) {
			return err
		}
		s.disk -= item.ref.Meta.Size
	}
	s.memory -= int64(len(item.content))
	delete(s.items, ref.ID)
	return nil
}

func (s *Store) ReadContent(ctx context.Context, ref Ref) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	item, ok := s.items[ref.ID]
	if !ok || item.ref != ref {
		return nil, ErrStale
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !item.ref.HasContent {
		return nil, ErrNoContent
	}
	if item.file == "" {
		return append([]byte{}, item.content...), nil
	}
	file, err := os.Open(item.file)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// The owned file is bounded by MaxFileBytes; never use caller-supplied sizes.
	data, err := readBounded(ctx, file, item.ref.Meta.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != item.ref.Meta.Size {
		return nil, fmt.Errorf("snapshot cache was truncated")
	}
	return data, nil
}

func (s *Store) Close() error {
	s.cancel() // cancel active reads/capture before waiting for ownership
	<-s.op
	defer s.release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	var first error
	for _, dir := range append(s.retired, s.dir) {
		if dir != "" {
			if err := os.RemoveAll(filepath.Clean(dir)); err != nil && first == nil {
				first = err
			}
		}
	}
	s.items = nil
	s.baseline.Files = nil
	s.memory = 0
	s.disk = 0
	s.closeErr = first
	return first
}

// Waiting for another capture/reset is itself cancellable.
func (s *Store) acquire(ctx context.Context) error {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.op:
		return nil
	}
}
func (s *Store) release() { s.op <- struct{}{} }

var _ SnapshotStore = (*Store)(nil)
