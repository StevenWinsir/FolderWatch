//go:build unix

package changes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnreadableSubtreeIsNotMassDeleted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix permission bits")
	}
	root := t.TempDir()
	put(t, root, "locked/a", "before")
	put(t, root, "healthy", "before")
	s := fixture(t, root, nil)
	put(t, root, "locked/a", "changed")
	resolve(t, s, "locked/a")
	dir := filepath.Join(root, "locked")
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	put(t, root, "healthy", "changed")
	_, warnings, err := s.ResolveBatch(context.Background(), nil, true)
	if err != nil || len(warnings) == 0 {
		t.Fatalf("missing warning %v %+v", err, warnings)
	}
	wantKinds(t, s, map[string]Kind{"locked/a": Modified, "healthy": Modified})
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, root, "locked/a", "before")
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{"healthy": Modified})
}
