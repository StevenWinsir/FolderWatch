package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// This deliberately exercises the native library BEFORE our path filter, so
// merely hiding out-of-root events cannot make the no-follow regression pass.
func TestNativeDirectoryWatchDoesNotFollowSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	put(t, filepath.Join(outside, "existing"), "outside baseline")
	if err := os.Symlink(outside, filepath.Join(root, "directory-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "existing"), filepath.Join(root, "file-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(root); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(outside, "existing"), "outside edit")
	put(t, filepath.Join(outside, "new"), "outside creation")
	select {
	case e := <-w.Events:
		t.Fatalf("native library followed symlink: %+v", e)
	case err := <-w.Errors:
		t.Fatal(err)
	case <-time.After(180 * time.Millisecond):
	}
}

func TestEmptyNativeNameIsNotAPath(t *testing.T) {
	w := &FSNotify{}
	if err := w.handle(context.Background(), fsnotify.Event{Op: fsnotify.Remove}); err != nil || !w.dirty {
		t.Fatalf("unknown event err=%v dirty=%v", err, w.dirty)
	}
}

func TestDeletedRecreatedDirectoryGetsNewRegistration(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "same")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	w, ch, errs := setup(t, root, 128, 64)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Consume structural invalidations like the application does; recovery must
	// finish before asserting a later independent file edit is watched.
	settle(t, w, ch, errs)
	put(t, filepath.Join(dir, "fresh"), "fresh")
	await(t, ch, errs, map[string]Op{"same/fresh": Create | Write})
}

func settle(t *testing.T, w *FSNotify, ch <-chan RawEvent, errs <-chan error) {
	t.Helper()
	quiet := time.NewTimer(70 * time.Millisecond)
	defer quiet.Stop()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("watch closed while settling")
			}
			if e.Reconcile || e.IsDir {
				if err := w.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(70 * time.Millisecond)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if !errors.Is(err, ErrOverflow) && !errors.Is(err, fsnotify.ErrEventOverflow) {
				t.Fatal(err)
			}
			if err := w.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
		case <-quiet.C:
			return
		case <-deadline:
			t.Fatal("structural events did not settle")
		}
	}
}
