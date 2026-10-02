package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/eventnorm"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

func putSession(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func sessionFixture(t *testing.T, root string) *Session {
	t.Helper()
	delay := "20ms"
	prepared, err := Prepare(context.Background(), root, config.Overlay{Debounce: &delay}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := StartSession(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	e := eventNext(t, session)
	if e.Type != "ready" || e.Generation != 1 || !e.Reconcile {
		t.Fatalf("ready %+v", e)
	}
	return session
}
func eventNext(t *testing.T, s *Session) Event {
	t.Helper()
	select {
	case e, ok := <-s.Events():
		if !ok {
			t.Fatalf("closed: %v", s.Err())
		}
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("session event timeout")
		return Event{}
	}
}
func changedPath(t *testing.T, s *Session, path string) Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-s.Events():
			if !ok {
				t.Fatalf("closed: %v", s.Err())
			}
			if e.Reconcile {
				return e
			}
			for _, p := range e.Paths {
				if p.Path == path {
					return e
				}
			}
		case <-deadline:
			t.Fatalf("no invalidation for %s", path)
		}
	}
}

func TestSessionStableBaselineAndAtomicSave(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a.txt", "hello")
	putSession(t, root, ".folderwatchignore", "*.tmp\n")
	s := sessionFixture(t, root)
	first := s.Baseline().Files["a.txt"]
	for _, value := range []string{"hello1", "hello2", "hello"} {
		putSession(t, root, "a.txt", value)
		changedPath(t, s, "a.txt")
		baseline, err := s.ReadBaseline(context.Background(), "a.txt")
		if err != nil || string(baseline) != "hello" {
			t.Fatalf("baseline advanced %q %v", baseline, err)
		}
		if s.Baseline().Generation != 1 {
			t.Fatal("ordinary write changed generation")
		}
	}
	sum := sha256.Sum256([]byte("hello"))
	if first.Hash != hex.EncodeToString(sum[:]) {
		t.Fatal("restoration hash")
	}
	// Atomic replacement with an ignored temporary path must still invalidate a.txt.
	putSession(t, root, "a.tmp", "atomic result")
	if err := os.Rename(filepath.Join(root, "a.tmp"), filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	changedPath(t, s, "a.txt")
	got, err := s.ReadBaseline(context.Background(), "a.txt")
	if err != nil || string(got) != "hello" {
		t.Fatal("atomic save advanced baseline")
	}
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	changedPath(t, s, "a.txt")
	if _, err := s.ReadBaseline(context.Background(), "a.txt"); err != nil {
		t.Fatal("delete lost before content")
	}
	putSession(t, root, "a.txt", "reset state")
	putSession(t, root, "new.txt", "new file")
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	e := eventNext(t, s)
	if e.Type != "reset" || e.Generation != 2 || !e.Reconcile {
		t.Fatalf("reset marker %+v", e)
	}
	if _, err := s.snapshots.ReadContent(context.Background(), first); !errors.Is(err, snapshot.ErrStale) {
		t.Fatalf("old generation readable %v", err)
	}
	got, err = s.ReadBaseline(context.Background(), "a.txt")
	if err != nil || string(got) != "reset state" {
		t.Fatalf("reset baseline %q %v", got, err)
	}
	putSession(t, root, "a.txt", "after reset")
	e = changedPath(t, s, "a.txt")
	if e.Generation != 2 {
		t.Fatal("old generation event after reset")
	}
	if _, ok := s.Baseline().Files["new.txt"]; !ok {
		t.Fatal("new baseline omitted added file")
	}
	if _, err := s.ReadBaseline(context.Background(), "../escape"); err == nil {
		t.Fatal("baseline API traversal")
	}
}

func TestSessionNewDirectoryAndContinuousSaves(t *testing.T) {
	root := t.TempDir()
	s := sessionFixture(t, root)
	if err := os.MkdirAll(filepath.Join(root, "new", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "new/deep/a", "immediate")
	changedPath(t, s, "new/deep/a")
	// Directory invalidation is followed by application registration reconciliation.
	if err := s.watcher.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		putSession(t, root, "new/deep/a", "burst")
	}
	changedPath(t, s, "new/deep/a")
	// After settling, a later save in that directory remains watched.
	time.Sleep(80 * time.Millisecond)
	putSession(t, root, "new/deep/later", "later")
	changedPath(t, s, "new/deep/later")
	if s.Baseline().Generation != 1 {
		t.Fatal("baseline automatically advanced")
	}
}

func TestResetCancellationKeepsPreviousGeneration(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "original")
	s := sessionFixture(t, root)
	first := s.Baseline()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.ResetBaseline(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	large, err := os.Create(filepath.Join(root, "large"))
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(2 << 30); err != nil {
		large.Close()
		t.Fatal(err)
	}
	large.Close()
	timed, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancel()
	if err := s.ResetBaseline(timed); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed reset=%v", err)
	}
	if s.Baseline().Generation != first.Generation {
		t.Fatal("cancelled reset committed")
	}
	if got, err := s.ReadBaseline(context.Background(), "a"); err != nil || string(got) != "original" {
		t.Fatalf("old content %q %v", got, err)
	}
	if err := os.Remove(filepath.Join(root, "large")); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Baseline().Generation != 2 {
		t.Fatal("reset after cancellation failed")
	}
}

func TestConcurrentResetAndStopResourceCleanup(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("TMPDIR", cache)
	t.Setenv("TMP", cache)
	t.Setenv("TEMP", cache)
	before := runtime.NumGoroutine()
	root := t.TempDir()
	putSession(t, root, "a", "original")
	for i := 0; i < 12; i++ {
		s := sessionFixture(t, root)
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.ResetBaseline(context.Background()); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if s.Baseline().Generation != 5 {
			t.Fatal("concurrent resets not serialized")
		}
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.Close(); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if err := s.ResetBaseline(context.Background()); !errors.Is(err, ErrSessionClosed) {
			t.Fatalf("reset after stop %v", err)
		}
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if len(entry.Name()) >= 12 && entry.Name()[:12] == "folderwatch-" {
			t.Fatalf("leaked cache %s", entry.Name())
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+4 {
		t.Fatalf("goroutine growth before=%d after=%d", before, after)
	}
}

func TestPublicationBackpressureIsExplicit(t *testing.T) {
	s := &Session{events: make(chan Event, 1), generation: 4}
	s.publish(Event{Type: "paths", Paths: []eventnorm.Request{{Path: "a"}}})
	s.publish(Event{Type: "paths", Paths: []eventnorm.Request{{Path: "b"}}})
	e := <-s.events
	if !e.Reconcile || len(e.Paths) != 0 || e.Sequence != 2 || e.Generation != 4 {
		t.Fatalf("lost work %+v", e)
	}
}

func TestSessionRootRemovalAndParentCancellation(t *testing.T) {
	root := t.TempDir()
	s := sessionFixture(t, root)
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root + "-moved")
	select {
	case <-s.Done():
		if s.Err() == nil {
			t.Fatal("lost root without fatal error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root removal did not stop session")
	}
	root = t.TempDir()
	prepared, err := Prepare(context.Background(), root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err = StartSession(ctx, prepared)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("parent cancellation leaked")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("normal cancel %v", err)
	}
}
