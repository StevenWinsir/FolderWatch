package app

import (
	"errors"
	"fmt"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/debounce"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

func (s *Session) resolveChanges(input debounce.Batch) (retry bool, err error) {
	if input.Reconcile {
		if err := s.watcher.Reconcile(s.ctx); err != nil {
			if s.ctx.Err() != nil {
				return false, s.ctx.Err()
			}
			if errors.Is(err, watcher.ErrRootGone) || errors.Is(err, watcher.ErrClosed) || errors.Is(err, watcher.ErrDirectoryLimit) {
				return false, err
			}
			s.publish(Event{Type: "warning", Message: err.Error(), Reconcile: true})
			retry = true
		}
	}
	paths := make([]string, 0, len(input.Paths))
	for _, p := range input.Paths {
		paths = append(paths, p.Path)
	}
	batch, warnings, err := s.changes.ResolveBatch(s.ctx, paths, input.Reconcile)
	if err != nil {
		if s.ctx.Err() != nil || errors.Is(err, changes.ErrCapacity) || errors.Is(err, changes.ErrClosed) {
			return false, err
		}
		s.publish(Event{Type: "warning", Message: err.Error(), Reconcile: true})
		return true, nil
	}
	if len(warnings) > 0 {
		s.publish(Event{Type: "warning", Message: fmt.Sprintf("%d unresolved path(s); %s: %s", len(warnings), warnings[0].Path, warnings[0].Message), Reconcile: true})
		retry = true
	}
	s.publish(Event{Type: "changes", Batch: &batch, Paths: input.Paths, Reconcile: input.Reconcile})
	return retry, nil
}
