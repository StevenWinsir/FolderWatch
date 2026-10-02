package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBoundedRegularConfig(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "config")
	if err := os.WriteFile(file, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := ReadConfig(file, false)
	if err != nil || string(data) != "ok" {
		t.Fatalf("%q %v", data, err)
	}
	if _, err := ReadConfig(root, false); err == nil {
		t.Fatal("accepted directory")
	}
	f, err := os.OpenFile(file, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxConfigBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(file, false); err == nil {
		t.Fatal("accepted oversized config")
	}
}

func TestAutomaticSymlinkNotRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege varies")
	}
	root := t.TempDir()
	file, link := filepath.Join(root, "config"), filepath.Join(root, "link")
	if err := os.WriteFile(file, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(link, false); err == nil {
		t.Fatal("followed automatic symlink")
	}
	if b, err := ReadConfig(link, true); err != nil || string(b) != "ok" {
		t.Fatalf("explicit symlink: %q %v", b, err)
	}
}
