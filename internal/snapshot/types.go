// Package snapshot owns immutable bounded content references and atomic baselines.
package snapshot

import (
	"context"
	"errors"
	"fmt"

	"github.com/StevenWinsir/FolderWatch/internal/filemeta"
	"github.com/StevenWinsir/FolderWatch/internal/filetype"
	"github.com/StevenWinsir/FolderWatch/internal/model"
)

var (
	ErrClosed    = errors.New("snapshot store closed")
	ErrStale     = errors.New("snapshot reference is stale or does not belong to this store")
	ErrNoContent = errors.New("snapshot retains metadata/hash only")
	ErrUnstable  = errors.New("file changed while capturing snapshot")
	ErrCapacity  = errors.New("snapshot entry limit exceeded")
	ErrStorage   = errors.New("snapshot storage unavailable")
)

// Ref has no filesystem cache path. ReadContent resolves only owned IDs.
// Retention is a storage decision; Class describes these exact captured bytes.
type Ref struct {
	ID         string             `json:"id"`
	Meta       model.FileMeta     `json:"meta"`
	Hash       string             `json:"hash,omitempty"`
	HasContent bool               `json:"has_content"`
	Retention  string             `json:"retention"`
	Class      filetype.Result    `json:"classification"`
	Signature  filemeta.Signature `json:"-"`
}

type Baseline struct {
	Generation uint64         `json:"generation"`
	Files      map[string]Ref `json:"files"`
	Error      string         `json:"error,omitempty"`
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
	MaxFiles        int // 0 removes the reference-count cap, not content budgets.
	// TempDir defaults to the OS temporary directory. It must be outside Root.
	TempDir string
}

func (o Options) validate() error {
	if o.MaxFileBytes <= 0 || o.MemoryFileBytes <= 0 || o.MemoryBytes < 0 || o.DiskBytes < 0 || o.MaxFiles < 0 {
		return fmt.Errorf("snapshot limits require positive file byte caps and non-negative entry/content budgets (0 entries = no count cap)")
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
