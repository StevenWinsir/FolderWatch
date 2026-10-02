// Package model holds UI-independent domain metadata.
package model

import (
	"io/fs"
	"time"
)

type EntryKind string

const (
	Directory   EntryKind = "directory"
	RegularFile EntryKind = "file"
	Symlink     EntryKind = "symlink"
	Other       EntryKind = "other"
)

// FileMeta is Lstat metadata, NOT a snapshot or content classification.
// Path is canonical root-relative; root itself is ".". No file content is read.
type FileMeta struct {
	Path    string      `json:"path"`
	Kind    EntryKind   `json:"kind"`
	Size    int64       `json:"size"`
	Mode    fs.FileMode `json:"mode"`
	ModTime time.Time   `json:"mod_time"`
}
