package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
)

func adaptiveOptions(t *testing.T, root string) Options {
	t.Helper()
	m, err := ignore.New(ignore.Options{Root: root, Patterns: []string{"ignored/", "*.tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	return Options{Filter: m, EventBuffer: 64, MaxDirectories: 1}
}

func awaitFallback(t *testing.T, errs <-chan error) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case err, ok := <-errs:
			if !ok {
				t.Fatal("watching ended before fallback")
			}
			if errors.Is(err, ErrDirectoryLimit) {
				t.Fatal("recovered limit leaked as fatal")
			}
			if strings.Contains(err.Error(), "monitoring continues using periodic metadata scans") {
				return
			}
		case <-timer.C:
			t.Fatal("fallback was not made visible")
		}
	}
}

func TestAdaptiveNativeBudgetFallbackStartupAndRuntime(t *testing.T) {
	for _, startup := range []bool{true, false} {
		t.Run(map[bool]string{true: "startup", false: "runtime"}[startup], func(t *testing.T) {
			root := t.TempDir()
			if startup {
				if err := os.Mkdir(filepath.Join(root, "over"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			w, err := NewAdaptive(adaptiveOptions(t, root))
			if err != nil {
				t.Fatal(err)
			}
			w.factory = func(o Options) (Watcher, error) { return New(o) } // exercise real fsnotify budget even on FSEvents builds
			ch, errs, err := w.Start(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = w.Close() })
			if !startup {
				if err := os.Mkdir(filepath.Join(root, "over"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			awaitFallback(t, errs)
			// This occurs after fallback's initial metadata inventory, so only
			// continued monitoring can find it. No Reconcile call is used.
			put(t, filepath.Join(root, "over", "later.txt"), "observed by polling")
			await(t, ch, errs, map[string]Op{"over/later.txt": Create | Write})
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if err := w.Reconcile(context.Background()); !errors.Is(err, ErrClosed) {
				t.Fatal(err)
			}
		})
	}
}

type resourceFailure struct {
	events chan RawEvent
	errors chan error
	closed bool
}

func (f *resourceFailure) Start(context.Context, string) (<-chan RawEvent, <-chan error, error) {
	return f.events, f.errors, nil
}
func (f *resourceFailure) Reconcile(context.Context) error { return nil }
func (f *resourceFailure) Close() error {
	if !f.closed {
		f.closed = true
		close(f.events)
		close(f.errors)
	}
	return nil
}

func TestAdaptiveOSResourceErrorsReleaseNativeAndContinue(t *testing.T) {
	for _, failure := range []error{syscall.EMFILE, syscall.ENFILE, syscall.ENOSPC} {
		t.Run(failure.Error(), func(t *testing.T) {
			root := t.TempDir()
			w, err := NewAdaptive(adaptiveOptions(t, root))
			if err != nil {
				t.Fatal(err)
			}
			fake := &resourceFailure{events: make(chan RawEvent), errors: make(chan error, 1)}
			w.factory = func(Options) (Watcher, error) { return fake, nil }
			ch, errs, err := w.Start(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = w.Close() })
			fake.errors <- failure
			awaitFallback(t, errs)
			if !fake.closed {
				t.Fatal("native resources were not released before fallback warning")
			}
			put(t, filepath.Join(root, "later.txt"), "still watched")
			await(t, ch, errs, map[string]Op{"later.txt": Create | Write})
		})
	}
}

func TestPollingSameSizeRestoredMtimeAndSymlinkIsolation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(root, "a")
	put(t, path, "before")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	opts := adaptiveOptions(t, root)
	p := newPolling(opts)
	p.interval = 20 * time.Millisecond
	ch, errs, err := p.Start(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	sec, nsec := fileChangeTime(info)
	if sec == 0 && nsec == 0 {
		t.Skip("platform has no ctime fingerprint")
	}
	time.Sleep(5 * time.Millisecond)
	put(t, path, "after!")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(outside, "hidden"), "outside root")
	await(t, ch, errs, map[string]Op{"a": Write})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	for e := range ch {
		if strings.HasPrefix(e.Path, "link/") {
			t.Fatalf("followed symlink: %+v", e)
		}
	}
}
