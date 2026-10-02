package changes

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/filetype"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

// GetDiff reads at most one bounded current snapshot at a time. It validates
// current content against the resolved hash and rejects stale path versions or
// generations after computing. No permanent current-file copies or diff cache
// accumulate. The baseline is never recaptured from today's filesystem.
func (s *Store) GetDiff(ctx context.Context, path string) (out diff.Result, outErr error) {
	ctx, done := s.linked(ctx)
	defer done()
	if err := s.acquire(ctx, s.diffOp); err != nil {
		return diff.Result{}, err
	}
	defer func() { s.diffOp <- struct{}{} }()
	key, err := pathutil.Key(s.root, path)
	if err != nil {
		return diff.Result{}, err
	}
	s.mu.RLock()
	summary, exists := s.items[key]
	summary = copySummary(summary)
	generation := s.baseline.Generation
	beforeRef := s.baseline.Files[key]
	s.mu.RUnlock()
	if !exists {
		return diff.Result{}, ErrNotChanged
	}
	result := diff.Result{Path: key, Kind: string(summary.Kind), Status: diff.Text, Generation: generation, Version: summary.Version, Hunks: []diff.Hunk{}}
	if summary.Before != nil {
		result.BeforeHash = summary.Before.Hash
	}
	if summary.After != nil {
		result.AfterHash = summary.After.Hash
	}
	finish := func(r diff.Result) (diff.Result, error) {
		if err := ctx.Err(); err != nil {
			return diff.Result{}, err
		}
		s.mu.RLock()
		defer s.mu.RUnlock()
		current, ok := s.items[key]
		if !ok || s.baseline.Generation != generation || current.Version != summary.Version {
			return diff.Result{}, ErrStale
		}
		return r, nil
	}
	for _, st := range []*FileState{summary.Before, summary.After} {
		if st == nil {
			continue
		}
		if st.Class.Kind == filetype.TooLarge {
			result.Status = diff.TooLarge
			result.Reason = st.Class.Reason
			return finish(result)
		}
		if st.Meta.Size > s.opts.Diff.MaxBytes {
			result.Status = diff.TooLarge
			result.Reason = "diff byte limit"
			return finish(result)
		}
		if st.Class.Kind == filetype.UnsupportedText {
			result.Status = diff.Unsupported
			result.Reason = st.Class.Reason
			return finish(result)
		}
		if st.Class.Kind == filetype.Binary {
			result.Status = diff.Binary
			result.Reason = st.Class.Reason
			return finish(result)
		}
		if st.Class.Kind != filetype.Text || st.Meta.Kind != model.RegularFile {
			result.Status = diff.Unavailable
			result.Reason = "non-regular file; metadata only"
			return finish(result)
		}
	}
	if summary.Before != nil && !beforeRef.HasContent {
		result.Status = diff.Unavailable
		result.Reason = "baseline content was not retained"
		return finish(result)
	}
	var before, after []byte
	if summary.Before != nil {
		before, err = s.snapshots.ReadContent(ctx, beforeRef)
		if errors.Is(err, snapshot.ErrStale) {
			return diff.Result{}, ErrStale
		}
		if err != nil {
			return diff.Result{}, err
		}
	}
	if summary.After != nil {
		if err := pathutil.CheckParents(s.root, key); err != nil {
			return diff.Result{}, err
		}
		ref, err := s.diffScratch.Capture(ctx, key)
		if err != nil {
			return diff.Result{}, err
		}
		defer func() {
			if err := s.diffScratch.Delete(ref); err != nil {
				out = diff.Result{}
				outErr = errors.Join(outErr, err)
			}
		}()
		if !equalContent(summary.After, state(ref)) {
			return diff.Result{}, ErrStale
		}
		if !ref.HasContent {
			result.Status = diff.Unavailable
			result.Reason = "current content exceeds retention budget"
			return finish(result)
		}
		after, err = s.diffScratch.ReadContent(ctx, ref)
		if err != nil {
			return diff.Result{}, err
		}
	} else {
		absent, err := s.fileAbsent(key)
		if err != nil {
			return diff.Result{}, err
		}
		if !absent {
			return diff.Result{}, ErrStale
		}
	}
	computed, err := s.engine.Diff(ctx, before, after, s.opts.Diff)
	if err != nil {
		return diff.Result{}, err
	}
	result.Status, result.Reason, result.Hunks = computed.Status, computed.Reason, computed.Hunks
	return finish(result)
}

// A deleted descendant of a directory replaced by a file/link is absent in the
// monitored namespace. Never follow that new link just to confirm deletion.
func (s *Store) fileAbsent(key string) (bool, error) {
	if err := pathutil.CheckParents(s.root, "."); err != nil {
		return false, err
	}
	parts := strings.Split(key, "/")
	path := s.root
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if i < len(parts)-1 && !info.IsDir() {
			return true, nil
		}
		if i == len(parts)-1 {
			return info.IsDir(), nil
		}
	}
	return false, nil
}
