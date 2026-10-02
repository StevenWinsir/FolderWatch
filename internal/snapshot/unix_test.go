//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSymlinksAndFIFONeverReadTargets(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, outside, "secret", []byte("outside"))
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	ref, err := s.Capture(context.Background(), "link")
	if err != nil || ref.HasContent || ref.Hash == "" {
		t.Fatalf("link %+v %v", ref, err)
	}
	if file, err := openRegular(filepath.Join(root, "link")); err == nil {
		file.Close()
		t.Fatal("native open followed link")
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Capture(context.Background(), "dir/secret"); err == nil {
		t.Fatal("followed directory link")
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	ref, err = s.Capture(context.Background(), "pipe")
	if err != nil || ref.HasContent || ref.Retention != "metadata" {
		t.Fatalf("FIFO %+v %v", ref, err)
	}
}
