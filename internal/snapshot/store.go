package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/StevenWinsir/FolderWatch/internal/catalog"
	"github.com/StevenWinsir/FolderWatch/internal/filemeta"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

type stored struct {
	ref     Ref
	content []byte
	file    string
}

// Record contains metadata only. File is a private owned cache path, never IPC.
type Record struct {
	Ref    Ref
	File   string
	Pinned bool
}

// Store retains bounded file contents in RAM/disk and puts the unbounded-count
// metadata index on disk. Reset stages a separate generation before publication.
type Store struct {
	root         string
	policy       filemeta.Policy
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
	items        map[string]stored // nonempty in-memory contents only; at most 4096 entries
	records      *catalog.Store[Record]
	base         *catalog.Store[Ref]
	unknown      *catalog.Store[string]
	generation   uint64
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
	s := &Store{root: root, ctx: ctx, cancel: cancel, opts: opts, op: make(chan struct{}, 1), dir: dir, items: make(map[string]stored)}
	s.policy = filemeta.PolicyForRoot(root)
	s.op <- struct{}{}
	s.records, err = catalog.New[Record](dir)
	if err == nil {
		s.base, err = catalog.New[Ref](dir)
	}
	if err == nil {
		s.unknown, err = catalog.New[string](dir)
	}
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("snapshot metadata: %w", err)
	}
	return s, nil
}

func (s *Store) Root() string                                          { return s.root }
func (s *Store) CacheDir() string                                      { s.mu.RLock(); defer s.mu.RUnlock(); return s.dir }
func (s *Store) MetadataSignature(info fs.FileInfo) filemeta.Signature { return s.policy.Read(info) }

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
	if s.opts.MaxFiles > 0 && s.records.Len() >= s.opts.MaxFiles {
		return Ref{}, fmt.Errorf("%w: %d", ErrCapacity, s.opts.MaxFiles)
	}
	var item stored
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		item, err = s.capture(ctx, path)
		if !errors.Is(err, ErrUnstable) {
			break
		}
	}
	if err != nil {
		return Ref{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.records.Put(item.ref.ID, Record{Ref: item.ref, File: item.file}); err != nil {
		if item.file != "" {
			_ = os.Remove(item.file)
		}
		return Ref{}, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if len(item.content) > 0 {
		s.items[item.ref.ID] = item
		s.memory += int64(len(item.content))
	}
	if item.file != "" {
		s.disk += item.ref.Meta.Size
	}
	return item.ref, nil
}

// Head, Lookup and WalkBaseline are the hot-path API. Baseline is an explicit
// materializing compatibility/diagnostic view and must not be used by GUI polls.
func (s *Store) Head() (uint64, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.base == nil {
		return s.generation, 0
	}
	return s.generation, s.base.Len()
}
func (s *Store) Lookup(path string) (Ref, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Ref{}, false, ErrClosed
	}
	return s.base.Get(path)
}

// LookupVersioned binds a reference and its generation under the same lock.
// A reset may publish its snapshot before the semantic store publishes its head.
func (s *Store) LookupVersioned(path string) (Ref, bool, uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return Ref{}, false, s.generation, ErrClosed
	}
	ref, ok, err := s.base.Get(path)
	return ref, ok, s.generation, err
}
func (s *Store) Known(path string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false, ErrClosed
	}
	if ref, ok, err := s.base.Get(path); err != nil {
		return false, err
	} else if ok && ref.Meta.Kind != model.Directory {
		return true, nil
	}
	for key := path; ; {
		if _, ok, err := s.unknown.Get(key); ok || err != nil {
			return false, err
		}
		if key == "." {
			break
		}
		if at := strings.LastIndexByte(key, '/'); at >= 0 {
			key = key[:at]
		} else {
			key = "."
		}
	}
	return true, nil
}
func (s *Store) Coverage() (int, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.unknown == nil || s.closed {
		return 0, ""
	}
	page, err := s.unknown.Page(context.Background(), "", "")
	if err != nil {
		return s.unknown.Len(), err.Error()
	}
	if len(page) == 0 {
		return 0, ""
	}
	return s.unknown.Len(), page[0].Key + ": " + page[0].Value
}
func (s *Store) WalkUnknown(ctx context.Context, visit func(string, string) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	return s.unknown.Walk(ctx, "", visit)
}
func (s *Store) WalkBaseline(ctx context.Context, scope string, visit func(string, Ref) error) error {
	generation, _ := s.Head()
	prefix := ""
	if scope != "." && scope != "" {
		prefix = scope + "/"
		ref, ok, err := s.Lookup(scope)
		if err != nil {
			return err
		}
		if ok {
			if err := visit(scope, ref); err != nil {
				return err
			}
		}
	}
	after := ""
	for {
		s.mu.RLock()
		if s.closed || s.generation != generation {
			s.mu.RUnlock()
			return ErrStale
		}
		page, err := s.base.Page(ctx, after, prefix)
		s.mu.RUnlock()
		if err != nil {
			return err
		}
		for _, e := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(e.Key, e.Value); err != nil {
				return err
			}
		}
		if len(page) < catalog.PageSize {
			return ctx.Err()
		}
		after = page[len(page)-1].Key
	}
}
func (s *Store) Baseline() Baseline {
	g, _ := s.Head()
	out := Baseline{Generation: g, Files: make(map[string]Ref)}
	if err := s.WalkBaseline(context.Background(), ".", func(k string, r Ref) error { out.Files[k] = r; return nil }); err != nil {
		out.Error = err.Error()
	}
	return out
}
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	if s.records != nil && !s.closed {
		n = s.records.Len()
	}
	return Stats{References: n, MemoryBytes: s.memory, DiskBytes: s.disk}
}

// Source yields paths and failed scan scopes. Partial is permitted ONLY for the
// initial generation; unknown scopes remain explicitly unknown until a complete
// user-requested reset. Today's bytes never masquerade as a startup baseline.
type Source func(yield func(string) error, warning func(string, error) error) error

func (s *Store) Reset(ctx context.Context, files []string) error {
	if s.opts.MaxFiles > 0 && len(files) > s.opts.MaxFiles {
		return fmt.Errorf("%w: baseline exceeds snapshot entry limit %d", ErrCapacity, s.opts.MaxFiles)
	}
	return s.ResetFrom(ctx, func(yield func(string) error, _ func(string, error) error) error {
		for _, p := range files {
			if err := yield(p); err != nil {
				return err
			}
		}
		return nil
	}, false)
}
func (s *Store) ResetFrom(ctx context.Context, source Source, partial bool) (resultErr error) {
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
	if partial && s.generation != 0 {
		return errors.New("partial reset cannot replace an existing baseline")
	}
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
			resultErr = errors.Join(resultErr, fmt.Errorf("clean staging snapshot: %w", err))
		}
	}()
	warning := func(path string, cause error) error {
		if !partial || path == "." {
			return fmt.Errorf("baseline requires a complete scan: %s: %w", path, cause)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := pathutil.Key(s.root, path)
		if err != nil {
			return err
		}
		return stage.unknown.Put(key, cause.Error())
	}
	err = source(func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := pathutil.Key(s.root, path)
		if err != nil {
			return err
		}
		if _, ok, err := stage.base.Get(key); ok || err != nil {
			return err
		}
		ref, err := stage.Capture(ctx, key)
		if err != nil {
			if errors.Is(err, ErrCapacity) || errors.Is(err, ErrStorage) || ctx.Err() != nil {
				return err
			}
			return warning(key, err)
		}
		record, _, err := stage.records.Get(ref.ID)
		if err != nil {
			return err
		}
		record.Pinned = true
		if err := stage.records.Put(ref.ID, record); err != nil {
			return err
		}
		return stage.base.Put(key, ref)
	}, warning)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	oldDir, oldRecords, oldBase, oldUnknown := s.dir, s.records, s.base, s.unknown
	s.dir, s.records, s.base, s.unknown, s.items = stage.dir, stage.records, stage.base, stage.unknown, stage.items
	s.memory, s.disk, s.seq = stage.memory, stage.disk, stage.seq
	s.generation++
	s.mu.Unlock()
	stage.dir = ""
	stage.records = nil
	stage.base = nil
	stage.unknown = nil
	stage.items = nil
	stage.memory = 0
	stage.disk = 0
	// Publication already succeeded. Cleanup cannot make it an ambiguous failure.
	cleanupErr := errors.Join(oldRecords.Close(), oldBase.Close(), oldUnknown.Close())
	if cleanupErr != nil {
		s.closeErr = errors.Join(s.closeErr, cleanupErr)
	}
	if err := os.RemoveAll(oldDir); err != nil {
		s.retired = append(s.retired, oldDir)
	}
	return nil
}

func sameRef(a, b Ref) bool {
	// Gob retains timestamp values but not time.Time's private location pointer.
	sameTime := a.Meta.ModTime.Equal(b.Meta.ModTime)
	a.Meta.ModTime = b.Meta.ModTime
	return sameTime && a == b
}
func (s *Store) Delete(ref Ref) error {
	<-s.op
	defer s.release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	record, ok, err := s.records.Get(ref.ID)
	if err != nil {
		return err
	}
	if !ok || !sameRef(record.Ref, ref) {
		return ErrStale
	}
	if record.Pinned {
		return errors.New("cannot delete a baseline-owned reference; reset the baseline")
	}
	if record.File != "" {
		if err := os.Remove(record.File); err != nil && !os.IsNotExist(err) {
			return err
		}
		s.disk -= record.Ref.Meta.Size
	}
	if item, ok := s.items[ref.ID]; ok {
		s.memory -= int64(len(item.content))
		delete(s.items, ref.ID)
	}
	return s.records.Delete(ref.ID)
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
	record, ok, err := s.records.Get(ref.ID)
	if err != nil {
		return nil, err
	}
	if !ok || !sameRef(record.Ref, ref) {
		return nil, ErrStale
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !record.Ref.HasContent {
		return nil, ErrNoContent
	}
	if record.File == "" {
		if ref.Meta.Size == 0 {
			return []byte{}, nil
		}
		item, ok := s.items[ref.ID]
		if !ok {
			return nil, ErrNoContent
		}
		return append([]byte{}, item.content...), nil
	}
	file, err := os.Open(record.File)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := readBounded(ctx, file, record.Ref.Meta.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != record.Ref.Meta.Size {
		return nil, errors.New("snapshot cache was truncated")
	}
	return data, nil
}
func (s *Store) Close() error {
	s.cancel()
	<-s.op
	defer s.release()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	if s.records != nil {
		s.closeErr = errors.Join(s.closeErr, s.records.Close())
	}
	if s.base != nil {
		s.closeErr = errors.Join(s.closeErr, s.base.Close())
	}
	if s.unknown != nil {
		s.closeErr = errors.Join(s.closeErr, s.unknown.Close())
	}
	for _, dir := range append(s.retired, s.dir) {
		if dir != "" {
			s.closeErr = errors.Join(s.closeErr, os.RemoveAll(filepath.Clean(dir)))
		}
	}
	s.items = nil
	s.memory = 0
	s.disk = 0
	return s.closeErr
}
func (s *Store) acquire(ctx context.Context) error {
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
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
