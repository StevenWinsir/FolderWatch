package watcher

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestNativeSkippedSymlinksCanBeRemovedAndRecreated(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	native, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	if err := native.Add(root); err != nil {
		t.Fatal(err)
	}
	awaitNative := func(path string, op fsnotify.Op) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case e := <-native.Events:
				if e.Name == path && e.Has(op) {
					return
				}
			case err := <-native.Errors:
				t.Fatal(err)
			case <-deadline:
				t.Fatalf("missing native event %s %s", path, op)
			}
		}
	}
	kqueue := runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" || runtime.GOOS == "dragonfly"
	path := filepath.Join(root, "reused-link")
	for i := 0; i < 8; i++ {
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
		awaitNative(path, fsnotify.Create)
		// The parent's post-enrollment Write is a barrier before removing the link.
		if kqueue {
			awaitNative(root, fsnotify.Write)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if kqueue {
			awaitNative(root, fsnotify.Write)
		} else {
			awaitNative(path, fsnotify.Remove)
		}
	}
}
