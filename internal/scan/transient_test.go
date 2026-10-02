package scan

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestCancellationDuringFinalCallback(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filter := filterFunc(func(string, bool) (bool, error) { cancel(); return false, nil })
	_, err := Scan(ctx, root, filter)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("last callback cancellation was lost: %v", err)
	}
}

type disappearedEntry struct{ fs.DirEntry }

func (disappearedEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

func TestEntryDisappearsBetweenWalkAndStat(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "gone", nil)
	fixture(t, root, "keep", nil)
	walk := func(root string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if filepath.Base(p) == "gone" && err == nil {
				return fn(p, disappearedEntry{d}, nil)
			}
			return fn(p, d, err)
		})
	}
	result, err := scanWithWalker(context.Background(), root, matcher(t, root, false), walk)
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("transient: %+v %v", result, err)
	}
	if _, ok := entries(result)["keep"]; !ok {
		t.Fatal("lost surviving entry")
	}
	if _, ok := entries(result)["gone"]; ok {
		t.Fatal("included failed stat")
	}
}
