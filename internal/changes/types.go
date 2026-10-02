// Package changes owns current-vs-baseline semantics independently of any UI.
package changes

import (
	"errors"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/filetype"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

type Kind string

const (
	Added    Kind = "added"
	Modified Kind = "modified"
	Deleted  Kind = "deleted"
	Renamed  Kind = "renamed"
)

var (
	ErrClosed     = errors.New("change store closed")
	ErrStale      = errors.New("change or baseline changed during diff; retry with current state")
	ErrNotChanged = errors.New("path has no current semantic change")
	ErrCapacity   = errors.New("change inventory exceeds configured entry limit")
)

type FileState struct {
	Meta  model.FileMeta  `json:"meta"`
	Hash  string          `json:"hash,omitempty"`
	Class filetype.Result `json:"classification"`
}
type Summary struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"old_path,omitempty"`
	Kind      Kind       `json:"kind"`
	Before    *FileState `json:"before,omitempty"`
	After     *FileState `json:"after,omitempty"`
	FirstSeen time.Time  `json:"first_seen"`
	LastSeen  time.Time  `json:"last_seen"`
	Version   uint64     `json:"version"`
}
type View struct {
	Generation uint64    `json:"generation"`
	Version    uint64    `json:"version"`
	Changes    []Summary `json:"changes"`
}

// Batch is a versioned delta. Reload requires fetching View; it never means
// an empty list. No event includes file contents or private snapshot cache paths.
type Batch struct {
	Generation uint64    `json:"generation"`
	Version    uint64    `json:"version"`
	Upserts    []Summary `json:"upserts,omitempty"`
	Removed    []string  `json:"removed,omitempty"`
	Reload     bool      `json:"reload,omitempty"`
}

func (b Batch) Empty() bool { return !b.Reload && len(b.Upserts) == 0 && len(b.Removed) == 0 }

type Warning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
type Options struct {
	Snapshot   snapshot.Options
	Diff       diff.Options
	MaxEntries int
}

func state(ref snapshot.Ref) *FileState {
	return &FileState{Meta: ref.Meta, Hash: ref.Hash, Class: ref.Class}
}
func copySummary(s Summary) Summary {
	if s.Before != nil {
		v := *s.Before
		s.Before = &v
	}
	if s.After != nil {
		v := *s.After
		s.After = &v
	}
	return s
}

// EqualContent excludes mtime/chmod for regular files. Symlinks compare the
// link string hash, never their target bytes. Directories have no list entries.
func equalContent(a, b *FileState) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Meta.Kind != b.Meta.Kind {
		return false
	}
	if a.Meta.Kind == model.RegularFile || a.Meta.Kind == model.Symlink {
		return a.Hash == b.Hash
	}
	return a.Meta.Mode.Type() == b.Meta.Mode.Type() && a.Meta.Size == b.Meta.Size
}
