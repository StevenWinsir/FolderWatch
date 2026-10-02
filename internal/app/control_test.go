package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

func TestPauseFreezesStateAndResumeReconcilesNestedChanges(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "base")
	putSession(t, root, "deleted", "delete me")
	s := sessionFixture(t, root)
	putSession(t, root, "a", "edit")
	semanticWait(t, s, map[string]changes.Kind{"a": changes.Modified})
	if err := s.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Status() != Paused {
		t.Fatal(s.Status())
	}
	frozen := s.ChangeState()
	putSession(t, root, "a", "base")
	if err := os.Remove(filepath.Join(root, "deleted")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "new", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		putSession(t, root, "new/deep/added", "during pause")
	}
	time.Sleep(160 * time.Millisecond)
	if !reflect.DeepEqual(frozen, s.ChangeState()) {
		t.Fatal("paused semantic state moved")
	}
	if err := s.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Status() != Monitoring {
		t.Fatal(s.Status())
	}
	semanticWait(t, s, map[string]changes.Kind{"deleted": changes.Deleted, "new/deep/added": changes.Added})
	putSession(t, root, "new/deep/later", "still watched")
	semanticWait(t, s, map[string]changes.Kind{"deleted": changes.Deleted, "new/deep/added": changes.Added, "new/deep/later": changes.Added})
	if s.Baseline().Generation != 1 {
		t.Fatal("resume advanced baseline")
	}
}

func TestResetWhilePausedAndCancelledControls(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "base")
	s := sessionFixture(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Pause(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if s.Status() != Monitoring {
		t.Fatal(s.Status())
	}
	if err := s.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "a", "new base")
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Status() != Paused || len(s.Changes()) != 0 || s.Baseline().Generation != 2 {
		t.Fatal(s.Status(), s.ChangeState())
	}
	putSession(t, root, "a", "next")
	if err := s.Resume(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if s.Status() != Paused {
		t.Fatal("cancelled resume changed status")
	}
	if err := s.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{"a": changes.Modified})
	before, err := s.ReadBaseline(context.Background(), "a")
	if err != nil || string(before) != "new base" {
		t.Fatal(string(before), err)
	}
	if err := s.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.Status() != Idle {
		t.Fatal(s.Status())
	}
	for _, fn := range []func(context.Context) error{s.Pause, s.Resume, s.ResetBaseline} {
		if err := fn(context.Background()); !errors.Is(err, ErrSessionClosed) {
			t.Fatal(err)
		}
	}
}

func TestPausedRootLossRemainsFatal(t *testing.T) {
	root := t.TempDir()
	s := sessionFixture(t, root)
	if err := s.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
		if s.Err() == nil || s.Status() != Error {
			t.Fatal(s.Err(), s.Status())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pause suppressed fatal root loss")
	}
}

func TestConcurrentControlsAndStop(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "base")
	s := sessionFixture(t, root)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn := []func(context.Context) error{s.Pause, s.Resume, s.ResetBaseline}[i%3]
			err := fn(context.Background())
			if err != nil && !errors.Is(err, ErrSessionClosed) && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		}(i)
	}
	wg.Add(1)
	go func() { defer wg.Done(); _ = s.Close() }()
	wg.Wait()
}
