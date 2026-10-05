package changes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/model"
)

func TestScopedRecoveryDoesNotInspectUnrelatedSubtree(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"left", "right"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	put(t, root, "left/a", "baseline-left")
	put(t, root, "right/b", "baseline-right")
	s := fixture(t, root, func(o *Options) { o.MaxEntries = 0; o.Snapshot.MaxFiles = 0 })
	put(t, root, "left/a", "left-changed")
	put(t, root, "right/b", "right-changed")
	batch, warnings, err := s.ResolveScoped(context.Background(), nil, []string{"left"})
	if err != nil || len(warnings) != 0 || len(batch.Upserts) != 1 || batch.Upserts[0].Path != "left/a" {
		t.Fatal(batch, warnings, err)
	}
	if len(s.View().Changes) != 1 {
		t.Fatal("unrelated right tree resolved")
	}
	if _, warnings, err := s.ResolveBatch(context.Background(), nil, true); err != nil || len(warnings) != 0 {
		t.Fatal(warnings, err)
	}
	if len(s.View().Changes) != 2 {
		t.Fatal("full recovery missed independent right edit")
	}
}

func TestRecoveryDetectsSameSizeRestoredMtime(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "before")
	s := fixture(t, root, nil)
	path := filepath.Join(root, "a")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "a", "after!")
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	batch, warnings, err := s.ResolveBatch(context.Background(), nil, true)
	if err != nil || len(warnings) != 0 || len(batch.Upserts) != 1 {
		t.Fatal(batch, warnings, err)
	}
	if batch.Upserts[0].After.Meta.Kind != model.RegularFile || batch.Upserts[0].Kind != Modified {
		t.Fatal(batch)
	}
}

func TestDiffRejectsSnapshotPublishedBeforeSemanticGeneration(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "startup baseline")
	s := fixture(t, root, nil)
	put(t, root, "a", "first edit")
	if _, err := s.Resolve(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	// Exercise the reset publication interval deterministically: the snapshot
	// has advanced, while the semantic head is deliberately still the old one.
	if err := s.snapshots.Reset(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDiff(context.Background(), "a"); !errors.Is(err, ErrStale) {
		t.Fatalf("old summary accepted a new-generation before reference: %v", err)
	}
}
