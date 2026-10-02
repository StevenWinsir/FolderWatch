package eventnorm

import (
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

type fuzzFilter struct{}

func (fuzzFilter) Match(string, bool) (bool, error) { return false, nil }

func FuzzNormalize(f *testing.F) {
	root := f.TempDir()
	f.Add("a.txt", uint8(watcher.Write), false, false)
	f.Add("../escape", uint8(watcher.Create), false, false)
	f.Add("项目/space name", uint8(watcher.Chmod), false, false)
	f.Add(".", uint8(0), true, true)
	f.Fuzz(func(t *testing.T, path string, op uint8, directory, reconcile bool) {
		if len(path) > 4096 {
			return
		}
		raw := watcher.RawEvent{Path: path, Op: watcher.Op(op), IsDir: directory, Reconcile: reconcile}
		event, ok, err := Normalize(root, fuzzFilter{}, raw)
		if err != nil || !ok {
			return
		}
		key, err := pathutil.Key(root, path)
		if err != nil {
			t.Fatalf("accepted escaping path %q", path)
		}
		if event.Reconcile {
			if !directory && !reconcile {
				t.Fatal("invented reconciliation")
			}
			return
		}
		if event.Path != key || event.MetadataOnly != (raw.Op == watcher.Chmod) {
			t.Fatalf("inconsistent canonical invalidation: %+v", event)
		}
		again, err := pathutil.Key(root, event.Path)
		if err != nil || again != key {
			t.Fatal("canonical key is not idempotent")
		}
	})
}
