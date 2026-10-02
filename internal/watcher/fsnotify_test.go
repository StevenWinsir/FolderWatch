package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/fsnotify/fsnotify"
)

func setup(t *testing.T, root string, buffer, dirs int) (*FSNotify, <-chan RawEvent, <-chan error) {
	t.Helper()
	m, err := ignore.New(ignore.Options{Root: root, Patterns: []string{"ignored/", "*.tmp"}, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(Options{Filter: m, EventBuffer: buffer, MaxDirectories: dirs})
	if err != nil {
		t.Fatal(err)
	}
	events, errs, err := w.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	return w, events, errs
}
func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func await(t *testing.T, ch <-chan RawEvent, errs <-chan error, wants map[string]Op) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for len(wants) > 0 {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("events closed")
			}
			t.Logf("raw %+v", e)
			if mask, ok := wants[e.Path]; ok && e.Op&mask != 0 {
				delete(wants, e.Path)
			}
		case err, ok := <-errs:
			if ok {
				if errors.Is(err, ErrOverflow) || errors.Is(err, fsnotify.ErrEventOverflow) {
					continue
				}
				t.Fatalf("watch error %v", err)
			}
			errs = nil
		case <-deadline:
			t.Fatalf("missing events %v", wants)
		}
	}
}

func TestRealRecursiveCreateWriteRenameRemove(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "existing"), 0700); err != nil {
		t.Fatal(err)
	}
	w, ch, errs := setup(t, root, 128, 64)
	a := filepath.Join(root, "existing", "你好 with spaces.txt")
	put(t, a, "start")
	await(t, ch, errs, map[string]Op{"existing/你好 with spaces.txt": Create | Write})
	put(t, a, "changed")
	await(t, ch, errs, map[string]Op{"existing/你好 with spaces.txt": Write})
	b := filepath.Join(root, "existing", "renamed.txt")
	if err := os.Rename(a, b); err != nil {
		t.Fatal(err)
	}
	await(t, ch, errs, map[string]Op{"existing/你好 with spaces.txt": Rename | Remove, "existing/renamed.txt": Create | Rename})
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	await(t, ch, errs, map[string]Op{"existing/renamed.txt": Remove})
	// New descendants already containing files must still enroll before later edits.
	if err := os.MkdirAll(filepath.Join(root, "new", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "new", "deep", "a"), "before enrollment")
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "new", "deep", "later"), "after enrollment")
	await(t, ch, errs, map[string]Op{"new/deep/later": Create | Write})
	if err := os.Rename(filepath.Join(root, "new"), filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	// During migration, root/subtree invalidations legitimately replace individual
	// creates. Once reconciled, an independent later write must still be watched.
	settle(t, w, ch, errs)
	put(t, filepath.Join(root, "moved", "deep", "last"), "new path")
	await(t, ch, errs, map[string]Op{"moved/deep/last": Create | Write})
	if err := os.RemoveAll(filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.native.WatchList() {
		if strings.Contains(p, "/moved") || strings.Contains(p, "/new") {
			t.Fatalf("stale watch %q", p)
		}
	}
}

func TestMovedTreeNestedIgnoreAndSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	w, ch, errs := setup(t, root, 128, 64)
	tree := filepath.Join(outside, "tree")
	if err := os.Mkdir(tree, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(tree, ".gitignore"), "*.secret\n")
	put(t, filepath.Join(tree, "keep"), "already present")
	if err := os.Rename(tree, filepath.Join(root, "tree")); err != nil {
		t.Fatal(err)
	}
	// Observe a subtree invalidation, proving existing children will be re-read.
	deadline := time.After(5 * time.Second)
	found := false
	for !found {
		select {
		case e := <-ch:
			found = e.Reconcile
		case err := <-errs:
			t.Fatal(err)
		case <-deadline:
			t.Fatal("no moved-tree reconciliation")
		}
	}
	if err := w.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "tree", "new.secret"), "ignored")
	put(t, filepath.Join(root, "tree", "new.tmp"), "ignored")
	if err := os.Mkdir(filepath.Join(root, "ignored"), 0700); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "ignored", "hidden"), "ignored")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(outside, "outside-write"), "not watched")
	put(t, filepath.Join(root, "tree", "visible"), "visible")
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	visible := false
	for {
		select {
		case e := <-ch:
			if strings.Contains(e.Path, "secret") || strings.HasSuffix(e.Path, ".tmp") || strings.HasPrefix(e.Path, "ignored/") || strings.HasPrefix(e.Path, "link/") {
				t.Fatalf("ignored/link event %+v", e)
			}
			if e.Path == "tree/visible" {
				visible = true
			}
		case err := <-errs:
			t.Fatal(err)
		case <-timer.C:
			if !visible {
				t.Fatal("visible file not watched")
			}
			return
		}
	}
}

func TestOverflowCloseAndLifecycle(t *testing.T) {
	root := t.TempDir()
	w, ch, _ := setup(t, root, 1, 64)
	for i := 0; i < 100; i++ {
		put(t, filepath.Join(root, fmtName(i)), "burst")
	}
	// A command barrier must complete with an unread, saturated event channel.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	found := false
	deadline := time.After(5 * time.Second)
	for !found {
		select {
		case e := <-ch:
			found = e.Reconcile && e.Path == "."
		case <-deadline:
			t.Fatal("overflow was silently dropped")
		}
	}
	if _, _, err := w.Start(context.Background(), root); err == nil {
		t.Fatal("double start")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = w.Close() }()
	}
	wg.Wait()
	if err := w.Reconcile(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("reconcile after close %v", err)
	}
	for range ch {
	}
}
func fmtName(i int) string { return "file-" + string(rune(0x4e00+i)) }

func TestStartupLimitsAndRootRemoval(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := ignore.New(ignore.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	w, err := New(Options{Filter: m, EventBuffer: 2, MaxDirectories: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.Start(context.Background(), root); err == nil {
		w.Close()
		t.Fatal("directory limit ignored")
	}
	w.Close()
	before, err := New(Options{Filter: m, EventBuffer: 2, MaxDirectories: 10})
	if err != nil {
		t.Fatal(err)
	}
	before.Close()
	if _, _, err := before.Start(context.Background(), root); !errors.Is(err, ErrClosed) {
		t.Fatalf("close-before-start %v", err)
	}
	w, _, errs := setup(t, root, 32, 64)
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root + "-moved") })
	deadline := time.After(5 * time.Second)
	for {
		select {
		case err, ok := <-errs:
			if !ok {
				t.Fatal("root removal not diagnosed")
			}
			if errors.Is(err, ErrRootGone) {
				return
			}
		case <-deadline:
			w.Close()
			t.Fatal("root rename not detected")
		}
	}
}
