package changes

import (
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
)

func TestCannotMixBaselineOrMatcherFromAnotherRoot(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "base")
	s := fixture(t, root, nil)
	other := t.TempDir()
	foreign, err := ignore.New(ignore.Options{Root: other})
	if err != nil {
		t.Fatal(err)
	}
	if bad, err := New(other, foreign, s.snapshots, s.opts); err == nil {
		bad.Close()
		t.Fatal("foreign baseline accepted")
	}
	if bad, err := New(root, foreign, s.snapshots, s.opts); err == nil {
		bad.Close()
		t.Fatal("foreign matcher accepted")
	}
}
