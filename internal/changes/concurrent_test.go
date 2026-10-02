package changes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

type gatedEngine struct {
	entered chan struct{}
	release chan struct{}
}

func (g gatedEngine) Diff(ctx context.Context, a, b []byte, o diff.Options) (diff.Result, error) {
	close(g.entered)
	select {
	case <-ctx.Done():
		return diff.Result{}, ctx.Err()
	case <-g.release:
	}
	return (diff.BoundedLCS{}).Diff(ctx, a, b, o)
}

func TestStaleDiffDoesNotBlockResolvingOrReset(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(map[bool]string{false: "resolve", true: "reset"}[reset], func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "a", "before")
			s := fixture(t, root, nil)
			put(t, root, "a", "after")
			resolve(t, s, "a")
			gate := gatedEngine{make(chan struct{}), make(chan struct{})}
			s.engine = gate
			done := make(chan error, 1)
			go func() { _, err := s.GetDiff(context.Background(), "a"); done <- err }()
			select {
			case <-gate.entered:
			case <-time.After(time.Second):
				t.Fatal("diff not entered")
			}
			// A second diff waits on a cancellable token rather than spawning a worker.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			if _, err := s.GetDiff(ctx, "a"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			cancel()
			changed := make(chan error, 1)
			go func() {
				if reset {
					_, err := s.Reset(context.Background(), []string{"a"})
					changed <- err
				} else {
					if err := os.WriteFile(filepath.Join(root, "a"), []byte("latest"), 0600); err != nil {
						changed <- err
						return
					}
					_, err := s.Resolve(context.Background(), "a")
					changed <- err
				}
			}()
			select {
			case err := <-changed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("diff blocked resolving/reset")
			}
			close(gate.release)
			if err := <-done; !errors.Is(err, ErrStale) {
				t.Fatalf("stale diff accepted: %v", err)
			}
			if s.diffScratch.Stats().References != 0 {
				t.Fatal("diff reference leaked")
			}
		})
	}
}

func TestConcurrentReadersResolveAndReset(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "before")
	s := fixture(t, root, nil)
	put(t, root, "a", "after")
	resolve(t, s, "a")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				view := s.View()
				if len(view.Changes) > 1 {
					t.Error("duplicate path")
				}
				_, err := s.GetDiff(context.Background(), "a")
				if err != nil && !errors.Is(err, ErrStale) && !errors.Is(err, ErrNotChanged) {
					t.Error(err)
				}
			}
		}()
	}
	for i := 0; i < 12; i++ {
		if _, err := s.Reset(context.Background(), []string{"a"}); err != nil {
			t.Fatal(err)
		}
		resolve(t, s, "a")
	}
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDiff(context.Background(), "a"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
