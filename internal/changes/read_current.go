package changes

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

// Native file events force verification. Recovery may reuse a hash only when
// identity, size, mode, nanosecond mtime AND ctime are unchanged. A restored
// mtime alone is never sufficient, and unsupported platforms always read.
func (s *Store) readCurrent(ctx context.Context, key string, force bool, baseline snapshot.Ref, old Summary) (*FileState, error) {
	if err := pathutil.CheckParents(s.root, key); err != nil {
		if absent, e := s.fileAbsent(key); e == nil && absent {
			return nil, nil
		}
		return nil, err
	}
	info, err := os.Lstat(filepath.Join(s.root, filepath.FromSlash(key)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ignored, err := s.filter.Match(key, info.IsDir())
	if err != nil {
		return nil, err
	}
	if ignored {
		if baseline.ID != "" && baseline.Meta.Kind != model.Directory {
			return state(baseline), nil
		}
		return nil, nil
	}
	if info.IsDir() {
		return nil, nil
	}
	signature := s.resolveScratch.MetadataSignature(info)
	if !force && info.Mode().IsRegular() {
		if old.After != nil && old.After.Signature.Same(signature) {
			return old.After, nil
		}
		if baseline.ID != "" && baseline.Signature.Same(signature) {
			return state(baseline), nil
		}
	}
	ref, err := s.resolveScratch.Capture(ctx, key)
	if err != nil {
		return nil, err
	}
	after := state(ref)
	if err := s.resolveScratch.Delete(ref); err != nil {
		return nil, err
	}
	return after, nil
}
