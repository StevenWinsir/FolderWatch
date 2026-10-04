//go:build darwin && cgo

package watcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/testutil"
)

func TestFSEventsRecursiveCoverageUsesConstantDescriptors(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 128; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d-%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	before := testutil.OpenDescriptors()
	w, err := preferredWatcher(adaptiveOptions(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := w.(*FSEvents); !ok {
		t.Fatal("macOS did not select FSEvents")
	}
	ch, errs, err := w.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	after := testutil.OpenDescriptors()
	if before >= 0 && after > before+16 {
		t.Fatalf("per-directory descriptors: %d -> %d", before, after)
	}
	put(t, filepath.Join(root, "d-127", "late.txt"), "recursive")
	await(t, ch, errs, map[string]Op{"d-127/late.txt": Create | Write})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if after := testutil.OpenDescriptors(); before >= 0 && after > before+16 {
		t.Fatalf("descriptor leak %d -> %d", before, after)
	}
}

func TestFSEventsDroppedFlagsAndBackpressureRetainReconciliation(t *testing.T) {
	root := t.TempDir()
	opts := adaptiveOptions(t, root)
	opts.EventBuffer = 1
	w := &FSEvents{opts: opts}
	ch, errs, err := w.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	// Dropped events are global even when the associated name is ignored or
	// cannot be canonicalized. A saturated public channel cannot block Close.
	w.incoming <- nativeEvent{filepath.Join(root, "ignored"), 0x4}
	select {
	case e := <-ch:
		if !e.Reconcile || e.Path != "." {
			t.Fatal(e)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("kernel overflow lost")
	}
	w.lost <- struct{}{}
	select {
	case e := <-ch:
		if !e.Reconcile || e.Path != "." {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("callback overflow lost")
	}
	for i := 0; i < 100; i++ {
		put(t, filepath.Join(root, fmt.Sprintf("f-%03d", i)), "burst")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := w.Reconcile(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed reconcile: %v", err)
	}
	for range ch {
	}
}

func TestFSEventsRepeatedCloseJoinsCallbacks(t *testing.T) {
	root := t.TempDir()
	before := testutil.OpenDescriptors()
	for i := 0; i < 20; i++ {
		w := &FSEvents{opts: adaptiveOptions(t, root)}
		if _, _, err := w.Start(context.Background(), root); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(root, "active"), fmt.Sprint(i))
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if after := testutil.OpenDescriptors(); before >= 0 && after > before+16 {
		t.Fatalf("FSEvents descriptors leaked: %d -> %d", before, after)
	}
}
