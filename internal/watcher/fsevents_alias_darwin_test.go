//go:build darwin && cgo

package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFSEventsObservesIndependentEditsThroughRootAliases(t *testing.T) {
	for _, names := range [][2]string{{"MiXeDRoot", "mixedroot"}, {"Róót", "Ro\u0301o\u0301t"}} {
		t.Run(names[0], func(t *testing.T) {
			parent := t.TempDir()
			actual, alias := filepath.Join(parent, names[0]), filepath.Join(parent, names[1])
			if err := os.Mkdir(actual, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(alias); os.IsNotExist(err) {
				t.Skip("filesystem does not alias these spellings")
			} else if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(actual, "independent.txt")
			put(t, target, "before")
			w := &FSEvents{opts: adaptiveOptions(t, alias)}
			ch, errs, err := w.Start(context.Background(), alias)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			// Drain setup notifications; only an independent file-path event below
			// counts. A generic root invalidation cannot make this test pass.
			settle := time.NewTimer(300 * time.Millisecond)
		drain:
			for {
				select {
				case <-settle.C:
					break drain
				case <-ch:
				case e := <-errs:
					t.Fatal(e)
				}
			}
			put(t, target, "independent later edit")
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for {
				select {
				case e, ok := <-ch:
					if !ok {
						t.Fatal("stream closed")
					}
					if e.Path == "independent.txt" && e.Op&Write != 0 {
						return
					}
				case e := <-errs:
					t.Fatal(e)
				case <-deadline.C:
					t.Fatalf("native stream missed the edit through %q", alias)
				}
			}
		})
	}
}

func TestFSEventsRootChangedIsFatalEvenWhenOriginalSpellingStillResolves(t *testing.T) {
	root := t.TempDir()
	w := &FSEvents{opts: adaptiveOptions(t, root)}
	_, errs, err := w.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// On a case/normalization-insensitive volume a spelling-only rename can
	// still Lstat to the same inode. The native root-move signal must not be
	// downgraded to an ordinary rescan and leave an obsolete event prefix.
	w.incoming <- nativeEvent{path: w.root, flags: 0x20}
	select {
	case err := <-errs:
		if !errors.Is(err, ErrRootGone) {
			t.Fatalf("root move was hidden: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root move was not surfaced")
	}
}

func TestFSEventsRealSpellingOnlyRootRenameStops(t *testing.T) {
	parent := t.TempDir()
	root, renamed := filepath.Join(parent, "WatchRoot"), filepath.Join(parent, "watchroot")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed); os.IsNotExist(err) {
		t.Skip("requires a case-insensitive filesystem")
	} else if err != nil {
		t.Fatal(err)
	}
	w := &FSEvents{opts: adaptiveOptions(t, root)}
	_, errs, err := w.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := os.Rename(root, renamed); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errs:
		if !errors.Is(err, ErrRootGone) {
			t.Fatalf("spelling-only root rename: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root rename left the stream using an obsolete prefix")
	}
}
