package filemeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUntrustedFilesystemAndDifferentDeviceNeverReuseHashes(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "file")
	if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	plain := Read(info)
	untrusted := (Policy{Device: plain.Device}).Read(info)
	if untrusted.Strong || untrusted.Same(untrusted) {
		t.Fatal("untrusted metadata reused as content proof")
	}
	foreign := (Policy{Device: plain.Device + 1, Trusted: true}).Read(info)
	if foreign.Strong {
		t.Fatal("nested mount inherited root trust")
	}
}
