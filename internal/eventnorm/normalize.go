// Package eventnorm turns OS-neutral notifications into paths to re-evaluate.
package eventnorm

import (
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

type Request struct {
	Path         string `json:"path"`
	MetadataOnly bool   `json:"metadata_only,omitempty"`
}

type Event struct {
	Request
	Reconcile bool
}

// Normalize intentionally does not derive Added/Deleted/Modified. A vanished
// path is still valid input. R3 must stat/hash after settling, not trust Op.
func Normalize(root string, filter ignore.Filter, raw watcher.RawEvent) (Event, bool, error) {
	key, err := pathutil.Key(root, raw.Path)
	if err != nil {
		return Event{}, false, err
	}
	if raw.Reconcile && key == "." {
		return Event{Reconcile: true}, true, nil
	}
	ignored, err := filter.Match(key, raw.IsDir)
	if err != nil || ignored {
		return Event{}, false, err
	}
	if raw.Reconcile || raw.IsDir {
		return Event{Request: Request{Path: key}, Reconcile: true}, true, nil
	}
	if raw.Op == 0 {
		return Event{}, false, nil
	}
	return Event{Request: Request{Path: key, MetadataOnly: raw.Op == watcher.Chmod}}, true, nil
}
