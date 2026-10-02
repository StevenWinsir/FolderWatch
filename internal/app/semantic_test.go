package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

func semanticWait(t *testing.T, s *Session, want map[string]changes.Kind) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := map[string]changes.Kind{}
		for _, item := range s.Changes() {
			got[item.Path] = item.Kind
		}
		if reflect.DeepEqual(got, want) {
			return
		}
		select {
		case <-s.Done():
			t.Fatalf("session stopped: %v", s.Err())
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("semantic state %+v want %v", s.ChangeState(), want)
}

// A matching change kind does not prove the same file version is still current:
// startup reconciliation can observe a truncate/write boundary. Exercise the
// documented caller contract: retry ONLY ErrStale within a deadline, retaining
// all final content/status assertions and surfacing every other error.
func awaitSemanticDiff(t *testing.T, s *Session, path string) (diff.Result, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		result, err := s.GetDiff(ctx, path)
		if !errors.Is(err, changes.ErrStale) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return diff.Result{}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestLiveSemanticChangesDiffRestoreResetAndIgnoredAtomicSave(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a.txt", "base\n")
	putSession(t, root, ".folderwatchignore", "*.tmp\n")
	s := sessionFixture(t, root)
	putSession(t, root, "a.txt", "edited\n")
	semanticWait(t, s, map[string]changes.Kind{"a.txt": changes.Modified})
	r, err := awaitSemanticDiff(t, s, "a.txt")
	if err != nil || r.Status != diff.Text || !strings.Contains(diff.Unified(r), "-base\n+edited\n") {
		t.Fatalf("live diff %+v %v", r, err)
	}
	putSession(t, root, "a.tmp", "base\n")
	if err := os.Rename(filepath.Join(root, "a.tmp"), filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{})
	if _, err := s.GetDiff(context.Background(), "a.txt"); !errors.Is(err, changes.ErrNotChanged) {
		t.Fatal(err)
	}
	putSession(t, root, "new.txt", "new\n")
	semanticWait(t, s, map[string]changes.Kind{"new.txt": changes.Added})
	if err := os.Remove(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{})
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{"a.txt": changes.Deleted})
	putSession(t, root, "a.txt", "other\n")
	semanticWait(t, s, map[string]changes.Kind{"a.txt": changes.Modified})
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{})
	putSession(t, root, "a.txt", "after reset\n")
	semanticWait(t, s, map[string]changes.Kind{"a.txt": changes.Modified})
	r, err = awaitSemanticDiff(t, s, "a.txt")
	if err != nil || r.Generation != 2 || !strings.Contains(diff.Unified(r), "-other\n") {
		t.Fatalf("new generation %+v %v", r, err)
	}
}

func TestLiveDirectoryReconciliationAndBinaryClassification(t *testing.T) {
	root := t.TempDir()
	s := sessionFixture(t, root)
	if err := os.MkdirAll(filepath.Join(root, "new", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "new/deep/file", "a\x00")
	semanticWait(t, s, map[string]changes.Kind{"new/deep/file": changes.Added})
	r, err := awaitSemanticDiff(t, s, "new/deep/file")
	if err != nil || r.Status != diff.Binary || len(r.Hunks) > 0 {
		t.Fatalf("binary %+v %v", r, err)
	}
	if err := os.RemoveAll(filepath.Join(root, "new")); err != nil {
		t.Fatal(err)
	}
	semanticWait(t, s, map[string]changes.Kind{})
}

func TestSlowConsumerCanReloadAuthoritativeState(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "base")
	s := sessionFixture(t, root)
	// Fill without consuming notifications. Each independent save reaches the core.
	for i := 0; i < 36; i++ {
		value := strings.Repeat("x", i+1)
		putSession(t, root, "a", value)
		deadline := time.Now().Add(2 * time.Second)
		for {
			list := s.Changes()
			if len(list) == 1 && list[0].After.Meta.Size == int64(len(value)) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("core blocked by consumer")
			}
			time.Sleep(3 * time.Millisecond)
		}
	}
	found := false
	for len(s.events) > 0 {
		e := <-s.events
		if e.Batch != nil && e.Batch.Reload {
			found = true
		}
	}
	if !found {
		t.Fatal("slow consumer never told to reload")
	}
	view := s.ChangeState()
	if len(view.Changes) != 1 || view.Changes[0].After.Meta.Size != 36 {
		t.Fatal(view)
	}
}

func TestIndependentClassificationAndDiffConfig(t *testing.T) {
	root := t.TempDir()
	putSession(t, root, "a", "initial")
	retention, limit := "1KiB", "8B"
	p, err := Prepare(context.Background(), root, config.Overlay{MaxSnapshotBytes: &retention, MaxDiffBytes: &limit}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartSession(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	putSession(t, root, "a", "longer than eight bytes")
	semanticWait(t, s, map[string]changes.Kind{"a": changes.Modified})
	r, err := awaitSemanticDiff(t, s, "a")
	if err != nil || r.Status != diff.TooLarge {
		t.Fatalf("diff cap %+v %v", r, err)
	}
	if s.Changes()[0].After.Class.Kind != "text" {
		t.Fatal("diff cap changed classification")
	}
}
