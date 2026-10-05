//go:build darwin || linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

func TestPartialBaselinePreservesUnknownAndHealthyCoverage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses fixture permissions")
	}
	root := t.TempDir()
	putSession(t, root, "readable.txt", "baseline")
	if err := os.Mkdir(filepath.Join(root, "restricted"), 0700); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "restricted/a.txt", "not-readable-at-startup")
	if err := os.Chmod(filepath.Join(root, "restricted"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "restricted"), 0700) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	prepared, err := PrepareSession(ctx, root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartSession(ctx, prepared)
	if err != nil {
		t.Fatalf("one unreadable child rejected healthy root: %v", err)
	}
	defer s.Close()
	if count, _ := s.BaselineCoverage(); count == 0 {
		t.Fatal("partial coverage was hidden")
	}
	generation, _ := s.ChangeHead()
	putSession(t, root, "readable.txt", "still-monitored")
	waitExactChange(t, s, "readable.txt", changes.Modified, "still-monitored", 8*time.Second)
	if err := s.ResetBaseline(ctx); err == nil {
		t.Fatal("incomplete reset replaced original baseline")
	}
	if current, _ := s.ChangeHead(); current != generation {
		t.Fatal("failed reset advanced generation")
	}
	if err := os.Chmod(filepath.Join(root, "restricted"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	waitExactChange(t, s, "restricted/a.txt", changes.Unknown, "not-readable-at-startup", 8*time.Second)
	result, err := s.GetDiff(ctx, "restricted/a.txt")
	if err != nil || result.Status != diff.Unavailable {
		t.Fatal("unknown startup bytes were fabricated", result, err)
	}
	if err := s.ResetBaseline(ctx); err != nil {
		t.Fatal(err)
	}
	if count, _ := s.BaselineCoverage(); count != 0 {
		t.Fatal("complete reset retained unknown baseline")
	}
	putSession(t, root, "restricted/a.txt", "edit-after-explicit-reset")
	waitExactChange(t, s, "restricted/a.txt", changes.Modified, "edit-after-explicit-reset", 8*time.Second)
	before, err := s.ReadBaseline(ctx, "restricted/a.txt")
	if err != nil || string(before) != "not-readable-at-startup" {
		t.Fatal("reset baseline incorrect", string(before), err)
	}
}
