// Package snapshot owns immutable bounded content references and atomic baselines.
package snapshot

import (
	"context"
	"errors"
	"fmt"

	"github.com/StevenWinsir/FolderWatch/internal/filetype"
	"github.com/StevenWinsir/FolderWatch/internal/model"
)

var (
	ErrClosed    = errors.New("snapshot store closed")
	ErrStale     = errors.New("snapshot reference is stale or does not belong to this store")
	ErrNoContent = errors.New("snapshot retains metadata/hash only")
	ErrUnstable  = errors.New("file changed while capturing snapshot")
)

// Ref has no filesystem cache path. ReadContent resolves only owned IDs.
// Retention is a storage decision; Class describes these exact captured bytes.
type Ref struct {
	ID         string          `json:"id"`
	Meta       model.FileMeta  `json:"meta"`
	Hash       string          `json:"hash,omitempty"`
	HasContent bool            `json:"has_content"`
	Retention  string          `json:"retention"`
	Class      filetype.Result `json:"classification"`
}

type Baseline struct {
	Generation uint64         `json:"generation"`
	Files      map[string]Ref `json:"files"`
}

type Stats struct {
	References  int
	MemoryBytes int64
	DiskBytes   int64
}

type Options struct {
	MaxFileBytes    int64
	MemoryFileBytes int64
	MemoryBytes     int64
	DiskBytes       int64
	MaxFiles        int
	// TempDir defaults to the OS temporary directory. It must be outside Root.
	TempDir string
}

func (o Options) validate() error {
	if o.MaxFileBytes <= 0 || o.MemoryFileBytes <= 0 || o.MemoryBytes < 0 || o.DiskBytes < 0 || o.MaxFiles < 1 {
		return fmt.Errorf("snapshot limits require positive file/entry caps and non-negative total budgets")
	}
	return nil
}

type SnapshotStore interface {
	Capture(context.Context, string) (Ref, error)
	ReadContent(context.Context, Ref) ([]byte, error)
	Delete(Ref) error
	Reset(context.Context, []string) error
	Baseline() Baseline
	Close() error
}
