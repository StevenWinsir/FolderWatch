package eventnorm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

func TestNormalizeInvalidations(t *testing.T) {
	root := t.TempDir()
	m, err := ignore.New(ignore.Options{Root: root, Patterns: []string{"*.tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw              watcher.RawEvent
		keep, meta, root bool
	}{
		{watcher.RawEvent{Path: "deleted", Op: watcher.Remove}, true, false, false},
		{watcher.RawEvent{Path: filepath.Join(root, "a"), Op: watcher.Chmod}, true, true, false},
		{watcher.RawEvent{Path: "a", Op: watcher.Write | watcher.Chmod}, true, false, false},
		{watcher.RawEvent{Path: "x.tmp", Op: watcher.Write}, false, false, false},
		{watcher.RawEvent{Path: "directory", Op: watcher.Create, IsDir: true}, true, false, true},
		{watcher.RawEvent{Path: ".", Reconcile: true}, true, false, true},
		{watcher.RawEvent{Path: "a"}, false, false, false},
	} {
		e, keep, err := Normalize(root, m, tc.raw)
		if err != nil || keep != tc.keep || e.MetadataOnly != tc.meta || e.Reconcile != tc.root {
			t.Fatalf("%+v -> %+v %v %v", tc.raw, e, keep, err)
		}
	}
	if _, _, err := Normalize(root, m, watcher.RawEvent{Path: "../outside", Op: watcher.Write}); err == nil {
		t.Fatal("accepted escape")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Normalize(root, m, watcher.RawEvent{Path: "link/file", Op: watcher.Create}); err == nil {
		t.Fatal("traversed link")
	}
}
