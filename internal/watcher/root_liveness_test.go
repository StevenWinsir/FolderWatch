package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/fsnotify/fsnotify"
)

// Root health must not depend on any backend notification or trigger semantic
// invalidations while the original directory remains intact.
func TestRootHealthWithoutNativeNotifications(t *testing.T) {
	root := t.TempDir()
	w, events, errs := setup(t, root, 8, 64)
	// Deliberately remove only the native subscription, leaving the adapter
	// alive. This reproduces a silent backend on every OS without relying on
	// the Linux-specific timing of DELETE_SELF with a retained directory fd.
	if err := w.native.Remove(w.root); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		t.Fatalf("root health must not publish periodic invalidations: %#v", e)
	case err := <-errs:
		t.Fatalf("intact root became unhealthy: %v", err)
	case <-time.After(1100 * time.Millisecond):
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errs:
		if !errors.Is(err, ErrRootGone) {
			t.Fatalf("silent removed root: got %v, want ErrRootGone", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("silent root loss was not detected by the identity health check")
	}
}

// Every root notification is an identity invalidation, even without Remove.
// This covers the event-driven path separately from the silent-backend case.
func TestRootNotificationChecksIdentity(t *testing.T) {
	for _, state := range []string{"intact", "removed", "replaced"} {
		t.Run(state, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "root")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			identity, err := os.Lstat(root)
			if err != nil {
				t.Fatal(err)
			}
			filter, err := ignore.New(ignore.Options{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			w := &FSNotify{root: root, identity: identity, opts: Options{Filter: filter}, dirs: map[string]os.FileInfo{root: identity}, events: make(chan RawEvent, 1)}
			switch state {
			case "removed":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
			case "replaced":
				// Keep the old inode alive so the replacement cannot reuse it.
				if err := os.Rename(root, filepath.Join(parent, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			err = w.handle(context.Background(), fsnotify.Event{Name: root, Op: fsnotify.Chmod})
			if state == "intact" {
				if err != nil {
					t.Fatalf("ordinary root chmod became fatal: %v", err)
				}
			} else if !errors.Is(err, ErrRootGone) {
				t.Fatalf("root %s reported only as Chmod: got %v, want ErrRootGone", state, err)
			}
		})
	}
}
