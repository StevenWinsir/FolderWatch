//go:build darwin

package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/testutil"
	"github.com/fsnotify/fsnotify"
)

func TestNativeCloseJoinsAndReleasesDescriptors(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 128; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f-%03d", i)), []byte("baseline"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := testutil.OpenDescriptors()
	if before < 0 {
		t.Fatal("native descriptor measurement unavailable")
	}
	for cycle := 0; cycle < 20; cycle++ {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Add(root); err != nil {
			_ = w.Close()
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := w.Close(); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		// Close must join the native reader, even without an Events consumer.
		select {
		case _, ok := <-w.Events:
			if ok {
				t.Fatal("event after Close")
			}
		default:
			t.Fatal("Close returned before reader stopped")
		}
		select {
		case _, ok := <-w.Errors:
			if ok {
				t.Fatal("error after Close")
			}
		default:
			t.Fatal("Errors remained open after Close")
		}
		if err := w.Add(root); !errors.Is(err, fsnotify.ErrClosed) {
			t.Fatalf("Add after Close: %v", err)
		}
	}
	if after := testutil.OpenDescriptors(); after < 0 || after > before+2 {
		t.Fatalf("native descriptors leaked: %d -> %d", before, after)
	}
}

func TestNativeConcurrentEnrollmentAndCloseReleasesDescriptors(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 128; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f-%03d", i)), []byte("baseline"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := testutil.OpenDescriptors()
	if before < 0 {
		t.Fatal("native descriptor measurement unavailable")
	}
	for cycle := 0; cycle < 20; cycle++ {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Add(root); err != nil && !errors.Is(err, fsnotify.ErrClosed) {
				t.Error(err)
			}
		}()
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}
	if after := testutil.OpenDescriptors(); after < 0 || after > before+2 {
		t.Fatalf("racing enrollment leaked: %d -> %d", before, after)
	}
}
