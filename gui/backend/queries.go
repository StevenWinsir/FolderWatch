package backend

import (
	"context"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

func (a *API) GetChanges(req ChangesRequest) ChangesReply {
	out := ChangesReply{Changes: []ChangeSummary{}, NextOffset: -1}
	_, s, err := a.session(req.SessionRequest)
	if err != nil {
		out.Error = problem(err)
		return out
	}
	if req.Limit == 0 {
		req.Limit = 200
	}
	if req.Limit < 1 || req.Limit > 500 || req.Offset < 0 || (req.Offset > 0 && (req.Generation == "" || req.Version == "")) {
		out.Error = problem(fault("INVALID_PAGE", "Use limit 1..500, a nonnegative offset, and generation/version for continuation pages."))
		return out
	}
	view := s.ChangeState()
	if (req.Generation != "" && req.Generation != decimal(view.Generation)) || (req.Version != "" && req.Version != decimal(view.Version)) {
		out.Error = problem(fault("STALE_VERSION", "The change list changed. Reload from its first page."))
		return out
	}
	if req.Offset > len(view.Changes) {
		out.Error = problem(fault("INVALID_PAGE", "Offset is beyond the change list."))
		return out
	}
	end := req.Offset + req.Limit
	if end > len(view.Changes) {
		end = len(view.Changes)
	}
	out.SessionID, out.Generation, out.Version = req.SessionID, decimal(view.Generation), decimal(view.Version)
	out.Total = len(view.Changes)
	if end < out.Total {
		out.NextOffset = end
	}
	for _, summary := range view.Changes[req.Offset:end] {
		out.Changes = append(out.Changes, ChangeSummary{Path: summary.Path, OldPath: summary.OldPath, Kind: string(summary.Kind), Version: decimal(summary.Version), Before: fileInfo(summary.Before), After: fileInfo(summary.After), FirstSeen: summary.FirstSeen.UTC().Format(time.RFC3339Nano), LastSeen: summary.LastSeen.UTC().Format(time.RFC3339Nano)})
	}
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	if _, err := a.f.sessionLocked(req.SessionRequest); err != nil {
		return ChangesReply{Changes: []ChangeSummary{}, NextOffset: -1, Error: problem(err)}
	}
	return out
}

func fileInfo(s *changes.FileState) *FileInfo {
	if s == nil {
		return nil
	}
	return &FileInfo{SizeBytes: strconv.FormatInt(s.Meta.Size, 10), Kind: string(s.Meta.Kind), Classification: string(s.Class.Kind), Reason: s.Class.Reason}
}

// Only canonical slash-separated root-relative keys returned by GetChanges are
// accepted. We reject rather than silently normalize traversal or absolute paths.
func safeKey(root, key string) error {
	if key == "" || key == "." || len(key) > 32768 || strings.ContainsAny(key, "\x00\\") || filepath.IsAbs(key) || filepath.VolumeName(key) != "" || path.Clean(key) != key {
		return fault("INVALID_PATH", "Use a canonical root-relative file path from the change list.")
	}
	for _, part := range strings.Split(key, "/") {
		if part == ".." || part == "." || part == "" {
			return fault("INVALID_PATH", "Path traversal is not allowed.")
		}
	}
	if _, err := pathutil.Key(root, key); err != nil {
		return fault("INVALID_PATH", "Path is outside the monitored root.")
	}
	// Do not resolve descendants with EvalSymlinks. Deleted paths remain usable;
	// final symlinks are metadata-only in the core and are never read through.
	if err := pathutil.CheckParents(root, key); err != nil {
		return fault("INVALID_PATH", "A path ancestor is unavailable or is a symbolic link.")
	}
	return nil
}

func (a *API) GetDiff(req DiffRequest) DiffReply {
	r, s, err := a.session(req.SessionRequest)
	if err != nil {
		return DiffReply{Error: problem(err)}
	}
	if !validCounter(req.Generation) || !validCounter(req.Version) {
		return DiffReply{Error: problem(fault("INVALID_VERSION", "Diff requires the baseline generation and selected path version."))}
	}
	if err := safeKey(r.root, req.Path); err != nil {
		return DiffReply{Error: problem(err)}
	}
	// Reject excess work immediately instead of accumulating IPC diff goroutines
	// waiting for the core's single on-demand diff token.
	select {
	case a.f.diffSlot <- struct{}{}:
		defer func() { <-a.f.diffSlot }()
	default:
		return DiffReply{Error: problem(fault("BUSY", "Another diff request is in progress."))}
	}
	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	defer cancel()
	result, err := s.GetDiff(ctx, req.Path)
	if err != nil {
		return DiffReply{Error: problem(err)}
	}
	if decimal(result.Generation) != req.Generation || decimal(result.Version) != req.Version {
		return DiffReply{Error: problem(fault("STALE_VERSION", "The selected file or baseline changed. Refresh the selection."))}
	}
	out := diffDTO(req.SessionID, result)
	a.f.mu.Lock()
	defer a.f.mu.Unlock()
	if _, err := a.f.sessionLocked(req.SessionRequest); err != nil {
		return DiffReply{Error: problem(err)}
	}
	// Fence reset/path changes that landed while the wire DTO was being built.
	view := s.ChangeState()
	if decimal(view.Generation) != req.Generation {
		return DiffReply{Error: problem(changes.ErrStale)}
	}
	current := false
	for _, summary := range view.Changes {
		if summary.Path == req.Path && decimal(summary.Version) == req.Version {
			current = true
			break
		}
	}
	if !current {
		return DiffReply{Error: problem(changes.ErrStale)}
	}
	return DiffReply{Diff: out}
}
