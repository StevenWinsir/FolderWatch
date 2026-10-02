package changes

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
)

func (s *Store) Resolve(ctx context.Context, path string) (Batch, error) {
	batch, warnings, err := s.ResolveBatch(ctx, []string{path}, false)
	if err == nil && len(warnings) > 0 {
		err = errors.New(warnings[0].Message)
	}
	return batch, err
}

// ResolveBatch commits all successful reads as one delta; an unreadable path
// retains its last known semantic state and returns a warning. A cancellation
// commits none of this batch. Full reconciliation includes baseline+changed
// paths so deletions and Added->Deleted cannot disappear from the workset.
func (s *Store) ResolveBatch(ctx context.Context, paths []string, reconcile bool) (Batch, []Warning, error) {
	ctx, done := s.linked(ctx)
	defer done()
	if len(paths) > s.opts.MaxEntries {
		return Batch{}, nil, ErrCapacity
	}
	if err := s.acquire(ctx, s.op); err != nil {
		return Batch{}, nil, err
	}
	defer func() { s.op <- struct{}{} }()
	var warnings []Warning
	keys := make(map[string]bool)
	var inventory map[string]model.FileMeta
	protected := func(key string) bool {
		for _, w := range warnings {
			if w.Path == "." || key == w.Path || strings.HasPrefix(key, w.Path+"/") {
				return true
			}
		}
		return false
	}
	if reconcile {
		result, err := scan.Scan(ctx, s.root, s.filter)
		if err != nil {
			return Batch{}, nil, err
		}
		if len(result.Entries) > s.opts.MaxEntries {
			return Batch{}, nil, ErrCapacity
		}
		inventory = make(map[string]model.FileMeta, len(result.Entries))
		for _, e := range result.Entries {
			inventory[e.Path] = e
			if e.Kind != model.Directory {
				keys[e.Path] = true
			}
		}
		for _, w := range result.Warnings {
			warnings = append(warnings, Warning{Path: w.Path, Message: w.Message})
		}
		for key, ref := range s.baseline.Files {
			if ref.Meta.Kind != model.Directory {
				keys[key] = true
			}
		}
		s.mu.RLock()
		for key := range s.items {
			keys[key] = true
		}
		s.mu.RUnlock()
	} else {
		for _, path := range paths {
			key, err := pathutil.Key(s.root, path)
			if err != nil {
				return Batch{}, nil, err
			}
			keys[key] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	updates := make(map[string]*FileState, len(keys))
	for _, key := range ordered {
		if err := ctx.Err(); err != nil {
			return Batch{}, warnings, err
		}
		if key == "." || protected(key) {
			continue
		}
		if reconcile {
			entry, exists := inventory[key]
			if !exists {
				absent, err := s.fileAbsent(key)
				if err != nil {
					warnings = append(warnings, Warning{key, err.Error()})
					continue
				}
				if absent {
					updates[key] = nil
					continue
				}
				// A present entry omitted by the walk may be ignored or newly
				// recreated. Apply the shared policy/read below, not a false deletion.
			} else if entry.Kind == model.Directory {
				updates[key] = nil
				continue
			}
		}
		if err := pathutil.CheckParents(s.root, key); err != nil {
			warnings = append(warnings, Warning{key, err.Error()})
			continue
		}
		info, err := os.Lstat(filepath.Join(s.root, filepath.FromSlash(key)))
		if errors.Is(err, fs.ErrNotExist) {
			updates[key] = nil
			continue
		}
		if err != nil {
			warnings = append(warnings, Warning{key, err.Error()})
			continue
		}
		ignored, err := s.filter.Match(key, info.IsDir())
		if err != nil {
			warnings = append(warnings, Warning{key, err.Error()})
			continue
		}
		if ignored {
			// A retired/recreated Ignore scope can exclude a previously tracked
			// path. Suppress it rather than inventing a deletion or retaining a
			// stale modified entry. This does not implement ignore-file hot reload.
			if ref, exists := s.baseline.Files[key]; exists && ref.Meta.Kind != model.Directory {
				updates[key] = state(ref)
			} else {
				updates[key] = nil
			}
			continue
		}
		if info.IsDir() {
			updates[key] = nil
			continue
		}
		ref, err := s.resolveScratch.Capture(ctx, key)
		if err != nil {
			warnings = append(warnings, Warning{key, err.Error()})
			continue
		}
		updates[key] = state(ref)
		if err := s.resolveScratch.Delete(ref); err != nil {
			return Batch{}, warnings, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Batch{}, warnings, err
	}
	batch, err := s.commit(ctx, ordered, updates)
	return batch, warnings, err
}

func (s *Store) commit(ctx context.Context, ordered []string, updates map[string]*FileState) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	batch := Batch{Generation: s.baseline.Generation, Version: s.version}
	now := time.Now()
	nextVersion := s.version + 1
	for _, key := range ordered {
		after, ok := updates[key]
		if !ok {
			continue
		}
		var before *FileState
		if ref, exists := s.baseline.Files[key]; exists && ref.Meta.Kind != model.Directory {
			before = state(ref)
		}
		old, had := s.items[key]
		if equalContent(before, after) {
			if had {
				batch.Removed = append(batch.Removed, key)
			}
			continue
		}
		kind := Modified
		if before == nil {
			kind = Added
		} else if after == nil {
			kind = Deleted
		}
		if had && old.Kind == kind && equalContent(old.After, after) {
			continue
		}
		item := Summary{Path: key, Kind: kind, Before: before, After: after, FirstSeen: now, LastSeen: now, Version: nextVersion}
		if had {
			item.FirstSeen = old.FirstSeen
		}
		batch.Upserts = append(batch.Upserts, item)
	}
	count := len(s.items) - len(batch.Removed)
	for _, item := range batch.Upserts {
		if _, exists := s.items[item.Path]; !exists {
			count++
		}
	}
	if count > s.opts.MaxEntries {
		return Batch{}, ErrCapacity
	}
	if batch.Empty() {
		return batch, nil
	}
	for _, key := range batch.Removed {
		delete(s.items, key)
	}
	for i, item := range batch.Upserts {
		s.items[item.Path] = copySummary(item)
		batch.Upserts[i] = copySummary(item)
	}
	s.version = nextVersion
	batch.Version = s.version
	return batch, nil
}
