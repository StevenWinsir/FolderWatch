package changes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
)

type hookFilter struct {
	base ignore.Filter
	hook func(string, bool) (bool, error)
}

func (f hookFilter) Match(path string, isDir bool) (bool, error) {
	if f.hook != nil {
		return f.hook(path, isDir)
	}
	return f.base.Match(path, isDir)
}

func TestDisappearanceDuringCaptureRetainsPriorStateUntilReconcile(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "base")
	s := fixture(t, root, nil)
	put(t, root, "a", "modified")
	resolve(t, s, "a")
	original := s.filter
	s.filter = hookFilter{hook: func(path string, isDir bool) (bool, error) {
		if path == "a" {
			return false, os.Remove(filepath.Join(root, "a"))
		}
		return original.Match(path, isDir)
	}}
	batch, warnings, err := s.ResolveBatch(context.Background(), []string{"a"}, false)
	if err != nil || len(warnings) != 1 || !batch.Empty() {
		t.Fatalf("transient %v %+v %+v", err, warnings, batch)
	}
	wantKinds(t, s, map[string]Kind{"a": Modified})
	s.filter = original
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{"a": Deleted})
}

func TestRecreatedIgnoreScopeDoesNotProduceIgnoredDeletion(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "base")
	s := fixture(t, root, nil)
	put(t, root, "a", "changed")
	resolve(t, s, "a")
	original := s.filter
	// Models a directory identity with a new Ignore policy; existing files are
	// omitted by WalkDir, but that is exclusion, not evidence of deletion.
	s.filter = hookFilter{hook: func(path string, isDir bool) (bool, error) {
		if filepath.Base(path) == "a" {
			return true, nil
		}
		return original.Match(path, isDir)
	}}
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{})
}
