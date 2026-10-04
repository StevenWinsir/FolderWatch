package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Reproduce a genuine unlink after enumeration and before metadata retrieval.
// Only owned test fixtures are changed; no sleeps are used to provoke the race.
type vanishingFilter struct {
	root  string
	armed atomic.Bool
}

func (f *vanishingFilter) Match(key string, isDir bool) (bool, error) {
	if key == "racy.txt" && f.armed.Swap(false) {
		if err := os.Remove(filepath.Join(f.root, key)); err != nil {
			return false, err
		}
	}
	return false, nil
}

func TestFallbackSurvivesVanishingChild(t *testing.T) {
	for _, startup := range []bool{true, false} {
		t.Run(fmt.Sprintf("startup=%t", startup), func(t *testing.T) {
			root := t.TempDir()
			put(t, filepath.Join(root, "racy.txt"), "temporary")
			filter := &vanishingFilter{root: root}
			filter.armed.Store(true)
			w, err := NewAdaptive(Options{Filter: filter, EventBuffer: 16, MaxDirectories: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			fake := &resourceFailure{events: make(chan RawEvent), errors: make(chan error, 1)}
			w.factory = func(Options) (Watcher, error) {
				if startup {
					return nil, syscall.EMFILE
				}
				return fake, nil
			}
			events, errs, err := w.Start(context.Background(), root)
			if err != nil {
				t.Fatalf("healthy root lost during fallback: %v", err)
			}
			if !startup {
				fake.errors <- syscall.EMFILE
			}
			awaitFallback(t, errs)
			if filter.armed.Load() {
				t.Fatal("race was not exercised")
			}
			put(t, filepath.Join(root, "independent.txt"), "after fallback")
			deadline := time.NewTimer(8 * time.Second)
			defer deadline.Stop()
			for {
				select {
				case event, ok := <-events:
					if !ok {
						t.Fatal("fallback terminated monitoring")
					}
					if event.Path == "independent.txt" {
						ctx, cancel := context.WithTimeout(context.Background(), time.Second)
						defer cancel()
						if err := w.Reconcile(ctx); err != nil {
							t.Fatal(err)
						}
						return
					}
				case err, ok := <-errs:
					if !ok {
						errs = nil
					} else {
						t.Logf("recoverable: %v", err)
					}
				case <-deadline.C:
					t.Fatal("independent later edit was not monitored")
				}
			}
		})
	}
}
