package changes

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/StevenWinsir/FolderWatch/internal/catalog"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/progress"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

func (s *Store) Resolve(ctx context.Context, path string) (Batch, error) {
	batch, warnings, err := s.ResolveBatch(ctx, []string{path}, false)
	if err == nil && len(warnings) > 0 {
		err = errors.New(warnings[0].Message)
	}
	return batch, err
}

func (s *Store) ResolveBatch(ctx context.Context, paths []string, reconcile bool) (Batch, []Warning, error) {
	var scopes []string
	if reconcile {
		scopes = []string{"."}
	}
	return s.ResolveScoped(ctx, paths, scopes)
}

// ResolveScoped stages a pass on disk before publishing. Directory events keep
// their subtree scope; only overflow/resume/startup recovery require a root pass.
// A cancelled pass publishes nothing. Unreadable paths retain their last state.
func (s *Store) ResolveScoped(ctx context.Context, paths, scopes []string) (Batch, []Warning, error) {
	ctx, done := s.linked(ctx)
	defer done()
	if s.opts.MaxEntries > 0 && len(paths) > s.opts.MaxEntries {
		return Batch{}, nil, ErrCapacity
	}
	if err := s.acquire(ctx, s.op); err != nil {
		return Batch{}, nil, err
	}
	defer func() { s.op <- struct{}{} }()
	var reporter *progress.Reporter
	if len(scopes) > 0 {
		reporter = progress.Start(ctx, "Reconciling changes")
		defer reporter.Finish()
	}
	pass, err := s.newPass(ctx, len(scopes) == 0 && len(paths) <= catalog.PageSize)
	if err != nil {
		return Batch{}, nil, err
	}
	defer pass.close()
	for _, path := range paths {
		key, err := pathutil.Key(s.root, path)
		if err != nil {
			return Batch{}, pass.warnings, err
		}
		if err := pass.resolve(key, true); err != nil {
			return Batch{}, pass.warnings, err
		}
	}
	scanned := 0
	for _, scope := range compactScopes(scopes) {
		key, err := pathutil.Key(s.root, scope)
		if err != nil {
			return Batch{}, pass.warnings, err
		}
		err = scan.Stream(ctx, s.root, key, s.resolveScratch.CacheDir(), s.filter, func(meta model.FileMeta, walkErr error) error {
			reporter.Step()
			if walkErr != nil {
				return pass.warn(meta.Path, walkErr)
			}
			scanned++
			if s.opts.MaxEntries > 0 && scanned > s.opts.MaxEntries {
				return ErrCapacity
			}
			return pass.resolve(meta.Path, false)
		})
		if err != nil {
			return Batch{}, pass.warnings, err
		}
		// Both baseline and changed paths matter: an added file deleted again
		// has no baseline entry, but its prior Added summary must disappear.
		if err := s.snapshots.WalkBaseline(ctx, key, func(path string, _ snapshot.Ref) error {
			return pass.resolve(path, false)
		}); err != nil {
			return Batch{}, pass.warnings, err
		}
		prefix := ""
		if key != "." {
			prefix = key + "/"
			if err := pass.resolve(key, false); err != nil {
				return Batch{}, pass.warnings, err
			}
		}
		if err := s.items.Walk(ctx, prefix, func(path string, _ Summary) error {
			return pass.resolve(path, false)
		}); err != nil {
			return Batch{}, pass.warnings, err
		}
	}
	batch, err := pass.commit()
	return batch, pass.warnings, err
}

func compactScopes(scopes []string) []string {
	ordered := append([]string(nil), scopes...)
	sort.Strings(ordered)
	out := make([]string, 0, len(ordered))
	for _, scope := range ordered {
		if scope == "." {
			return []string{"."}
		}
		if len(out) > 0 && (scope == out[len(out)-1] || strings.HasPrefix(scope, out[len(out)-1]+"/")) {
			continue
		}
		out = append(out, scope)
	}
	return out
}
