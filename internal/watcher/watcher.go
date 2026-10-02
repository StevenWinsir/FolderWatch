// Package watcher adapts filesystem notifications without exposing fsnotify to callers.
package watcher

import (
	"context"
	"errors"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
)

type Op uint8

const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Chmod
)

// RawEvent is a path invalidation, not an Added/Modified/Deleted verdict.
// Reconcile means individual events were folded or a subtree needs rescanning.
type RawEvent struct {
	Path      string
	Op        Op
	IsDir     bool
	Reconcile bool
	At        time.Time
}

var (
	ErrClosed         = errors.New("watcher closed")
	ErrRootGone       = errors.New("watched root was removed, replaced or renamed")
	ErrOverflow       = errors.New("watcher overflow: reconcile root")
	ErrDirectoryLimit = errors.New("watch directory limit exceeded")
)

type Watcher interface {
	Start(context.Context, string) (<-chan RawEvent, <-chan error, error)
	Reconcile(context.Context) error
	Close() error
}

type Options struct {
	Filter         ignore.Filter
	EventBuffer    int
	MaxDirectories int
}
