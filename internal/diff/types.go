// Package diff provides bounded, cancellable, UI-independent line diffs.
package diff

import (
	"context"
	"fmt"
)

type Status string

const (
	Text        Status = "text"
	Binary      Status = "binary"
	Unsupported Status = "unsupported-text"
	TooLarge    Status = "too-large"
	Unavailable Status = "unavailable"
)

type LineKind string

const (
	Added   LineKind = "added"
	Removed LineKind = "removed"
	Context LineKind = "context"
)

type Line struct {
	Kind      LineKind `json:"kind"`
	OldLine   int      `json:"old_line,omitempty"`
	NewLine   int      `json:"new_line,omitempty"`
	Text      string   `json:"text"`
	NoNewline bool     `json:"no_newline,omitempty"`
}
type Hunk struct {
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	Lines    []Line `json:"lines"`
}
type Result struct {
	Path       string `json:"path,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     Status `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Generation uint64 `json:"generation,omitempty"`
	Version    uint64 `json:"version,omitempty"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
	Hunks      []Hunk `json:"hunks"`
}
type Options struct {
	ContextLines int
	MaxBytes     int64
	MaxLines     int
	MaxWork      int64
}

func Defaults() Options {
	return Options{ContextLines: 3, MaxBytes: 5 << 20, MaxLines: 20000, MaxWork: 2000000}
}
func (o Options) Validate() error {
	if o.ContextLines < 0 || o.ContextLines > 1000 || o.MaxBytes < 1 || o.MaxBytes > 64<<20 || o.MaxLines < 1 || o.MaxLines > 200000 || o.MaxWork < 1 || o.MaxWork > 4000000 {
		return fmt.Errorf("diff limits: context 0..1000, bytes 1..64MiB, lines 1..200000, work 1..4000000")
	}
	return nil
}

type Engine interface {
	Diff(context.Context, []byte, []byte, Options) (Result, error)
}
