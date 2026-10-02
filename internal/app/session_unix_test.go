//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnreadableResetDoesNotSilentlyEraseBaseline(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode-000 permissions")
	}
	root := t.TempDir()
	putSession(t, root, "a", "original")
	s := sessionFixture(t, root)
	if err := os.Chmod(filepath.Join(root, "a"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(root, "a"), 0600)
	if err := s.ResetBaseline(context.Background()); err == nil {
		t.Fatal("unreadable baseline accepted")
	}
	if s.Baseline().Generation != 1 {
		t.Fatal("partial reset erased old generation")
	}
	if got, err := s.ReadBaseline(context.Background(), "a"); err != nil || string(got) != "original" {
		t.Fatalf("old content %q %v", got, err)
	}
	if err := os.Chmod(filepath.Join(root, "a"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
}
