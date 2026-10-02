package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

func TestMovedTreeImmediateAndLaterContentsRemainObservable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "before", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "before/deep/a", "baseline")
	s := sessionFixture(t, root)
	if err := os.Rename(filepath.Join(root, "before"), filepath.Join(root, "after")); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "after/deep/immediate", "during enrollment")
	changedPath(t, s, "after/deep/immediate") // subtree/root reconciliation is valid here
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadBaseline(context.Background(), "after/deep/immediate")
	if err != nil || string(got) != "during enrollment" {
		t.Fatalf("migration window lost file: %q %v", got, err)
	}
	time.Sleep(100 * time.Millisecond)
	putSession(t, root, "after/deep/later", "independent later edit")
	changedPath(t, s, "after/deep/later")
	if err := s.ResetBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Baseline().Files["before/deep/a"]; ok {
		t.Fatal("old directory paths persisted")
	}
	if _, ok := s.Baseline().Files["after/deep/later"]; !ok {
		t.Fatal("later edit not captured")
	}
}

func TestDirectoryLimitStopsInsteadOfPretendingFullCoverage(t *testing.T) {
	root := t.TempDir()
	limit := 1
	prepared, err := Prepare(context.Background(), root, config.Overlay{MaxWatchDirs: &limit}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartSession(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := os.Mkdir(filepath.Join(root, "over-limit"), 0700); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
		if s.Err() == nil {
			t.Fatal("limit failure was hidden")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("incomplete monitoring remained active")
	}
	// The exact native error may race stream closure; either must be fatal.
	if !errors.Is(s.Err(), watcher.ErrDirectoryLimit) {
		t.Logf("pipeline also observed closure: %v", s.Err())
	}
}
