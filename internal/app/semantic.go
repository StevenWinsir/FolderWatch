package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/debounce"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

func (s *Session) resolveChanges(input debounce.Batch) (retry bool, err error) {
	return s.resolveChangesContext(s.ctx, input)
}

func (s *Session) resolveChangesContext(ctx context.Context, input debounce.Batch) (retry bool, err error) {
	if input.Reconcile || len(input.Scopes) > 0 {
		started := time.Now()
		defer func() { s.reconcileCost = time.Since(started) }()
	}
	if input.Reconcile || len(input.Scopes) > 0 {
		if err := s.watcher.Reconcile(ctx); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
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
	scopes := input.Scopes
	if input.Reconcile {
		scopes = []string{"."}
	}
	batch, warnings, err := s.changes.ResolveScoped(ctx, paths, scopes)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, changes.ErrCapacity) || errors.Is(err, changes.ErrClosed) {
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
