//go:build darwin || linux

package scan

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/model"
)

func TestPermissionDeniedDirectoryContinues(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "locked/file", nil)
	fixture(t, root, "readable", nil)
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0700) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("privileged filesystem permits reading mode-000 directories; deterministic error test still runs")
	}
	result, err := Scan(context.Background(), root, matcher(t, root, false))
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("permission scan: %+v %v", result, err)
	}
	if _, ok := entries(result)["readable"]; !ok {
		t.Fatal("lost readable sibling")
	}
	if _, ok := entries(result)["locked"]; ok {
		t.Fatal("unreadable directory offered for watching")
	}
}

func TestFIFOIsMetadataOnly(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(context.Background(), root, matcher(t, root, false))
	if err != nil {
		t.Fatal(err)
	}
	if entries(result)["pipe"].Kind != model.Other {
		t.Fatalf("FIFO kind: %+v", result)
	}
}
