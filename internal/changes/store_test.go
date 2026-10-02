package changes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/filetype"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

func put(t *testing.T, root, name, text string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T, root string, modify func(*Options)) *Store {
	t.Helper()
	opts := Options{Snapshot: snapshot.Options{MaxFileBytes: 4096, MemoryFileBytes: 64, MemoryBytes: 4096, DiskBytes: 8192, MaxFiles: 100, TempDir: t.TempDir()}, Diff: diff.Defaults(), MaxEntries: 100}
	if modify != nil {
		modify(&opts)
	}
	filter, err := ignore.New(ignore.Options{Root: root, Patterns: []string{"*.tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := scan.Scan(context.Background(), root, filter)
	if err != nil {
		t.Fatal(err)
	}
	base, err := snapshot.New(root, opts.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.Close(); err != nil {
			t.Error(err)
		}
	})
	var paths []string
	for _, e := range inv.Entries {
		paths = append(paths, e.Path)
	}
	if err := base.Reset(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, filter, base, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}
func resolve(t *testing.T, s *Store, path string) Batch {
	t.Helper()
	batch, err := s.Resolve(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}
func reconcile(t *testing.T, s *Store) Batch {
	t.Helper()
	batch, warnings, err := s.ResolveBatch(context.Background(), nil, true)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("reconcile %v %+v", err, warnings)
	}
	return batch
}
func kinds(s *Store) map[string]Kind {
	out := map[string]Kind{}
	for _, c := range s.View().Changes {
		out[c.Path] = c.Kind
	}
	return out
}
func wantKinds(t *testing.T, s *Store, want map[string]Kind) {
	t.Helper()
	if got := kinds(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds=%v want=%v", got, want)
	}
}

func TestStateTransitionsAndNoPriorSaveBaseline(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "hello\n")
	s := fixture(t, root, nil)
	wantKinds(t, s, map[string]Kind{})
	for _, text := range []string{"hello1\n", "hello2\n"} {
		put(t, root, "a", text)
		batch := resolve(t, s, "a")
		if len(batch.Upserts) != 1 {
			t.Fatal(batch)
		}
		wantKinds(t, s, map[string]Kind{"a": Modified})
		r, err := s.GetDiff(context.Background(), "a")
		if err != nil {
			t.Fatal(err)
		}
		rendered := diff.Unified(r)
		if !strings.Contains(rendered, "-hello\n") || !strings.Contains(rendered, "+"+text) {
			t.Fatal(rendered)
		}
	}
	put(t, root, "a", "hello\n")
	batch := resolve(t, s, "a")
	if !reflect.DeepEqual(batch.Removed, []string{"a"}) {
		t.Fatal(batch)
	}
	wantKinds(t, s, map[string]Kind{})
	if _, err := s.GetDiff(context.Background(), "a"); !errors.Is(err, ErrNotChanged) {
		t.Fatal(err)
	}
	put(t, root, "new", "new\n")
	resolve(t, s, "new")
	wantKinds(t, s, map[string]Kind{"new": Added})
	if err := os.Remove(filepath.Join(root, "new")); err != nil {
		t.Fatal(err)
	}
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{})
	if err := os.Remove(filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	resolve(t, s, "a")
	wantKinds(t, s, map[string]Kind{"a": Deleted})
	r, err := s.GetDiff(context.Background(), "a")
	if err != nil || !strings.Contains(diff.Unified(r), "-hello\n") {
		t.Fatalf("%+v %v", r, err)
	}
	put(t, root, "a", "hello\n")
	resolve(t, s, "a")
	wantKinds(t, s, map[string]Kind{})
	if err := os.Remove(filepath.Join(root, "a")); err != nil {
		t.Fatal(err)
	}
	resolve(t, s, "a")
	put(t, root, "a", "different")
	resolve(t, s, "a")
	wantKinds(t, s, map[string]Kind{"a": Modified})
	first := s.View().Changes[0]
	if err := os.Chmod(filepath.Join(root, "a"), 0644); err != nil {
		t.Fatal(err)
	}
	if !resolve(t, s, "a").Empty() {
		t.Fatal("chmod-only or duplicate content published")
	}
	if s.View().Changes[0].FirstSeen != first.FirstSeen {
		t.Fatal("duplicate resolve reset first-seen")
	}
	if got := s.resolveScratch.Stats(); got.References != 0 || got.MemoryBytes != 0 || got.DiskBytes != 0 {
		t.Fatalf("live content retained %+v", got)
	}
}

func TestRenameFallbackAndDirectoryTypeChanges(t *testing.T) {
	root := t.TempDir()
	put(t, root, "old/file", "base\n")
	put(t, root, "a", "a\n")
	s := fixture(t, root, nil)
	if err := os.Rename(filepath.Join(root, "a"), filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{"a": Deleted, "b": Added})
	if err := os.RemoveAll(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	put(t, root, "old", "replaced directory")
	reconcile(t, s)
	wantKinds(t, s, map[string]Kind{"a": Deleted, "b": Added, "old": Added, "old/file": Deleted})
	r, err := s.GetDiff(context.Background(), "old/file")
	if err != nil || !strings.Contains(diff.Unified(r), "-base\n") {
		t.Fatalf("deleted descendant %+v %v", r, err)
	}
	put(t, root, "ignored.tmp", "ignored")
	reconcile(t, s)
	if _, ok := kinds(s)["ignored.tmp"]; ok {
		t.Fatal("ignored path entered change store")
	}
	if err := os.Remove(filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0700); err != nil {
		t.Fatal(err)
	}
	reconcile(t, s)
	if _, ok := kinds(s)["b"]; ok {
		t.Fatal("Added->directory should remove added file")
	}
}

func TestClassifiedStatesAndBudgetFallback(t *testing.T) {
	cases := []struct {
		name, before, after string
		want                diff.Status
		class               filetype.Kind
	}{
		{"binary", "a\x00", "b\x00", diff.Binary, filetype.Binary},
		{"utf16", "\xff\xfea\x00", "\xff\xfeb\x00", diff.Unsupported, filetype.UnsupportedText},
		{"size", strings.Repeat("a", 80), strings.Repeat("b", 80), diff.TooLarge, filetype.TooLarge},
		{"diff-smaller-than-snapshot", "123456789", "ABCDEFGHI", diff.TooLarge, filetype.Text},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "a", tc.before)
			s := fixture(t, root, func(o *Options) {
				o.Snapshot.MaxFileBytes = 64
				if tc.name == "diff-smaller-than-snapshot" {
					o.Diff.MaxBytes = 8
				}
			})
			put(t, root, "a", tc.after)
			resolve(t, s, "a")
			summary := s.View().Changes[0]
			if summary.After.Class.Kind != tc.class {
				t.Fatal(summary)
			}
			r, err := s.GetDiff(context.Background(), "a")
			if err != nil || r.Status != tc.want || len(r.Hunks) != 0 {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	root := t.TempDir()
	put(t, root, "a", "before")
	s := fixture(t, root, func(o *Options) { o.Snapshot.MemoryBytes = 0; o.Snapshot.DiskBytes = 0 })
	put(t, root, "a", "after")
	resolve(t, s, "a")
	r, err := s.GetDiff(context.Background(), "a")
	if err != nil || r.Status != diff.Unavailable {
		t.Fatalf("no baseline content %+v %v", r, err)
	}
}

func TestResetRollbackCopiesAndStaleCurrent(t *testing.T) {
	root := t.TempDir()
	put(t, root, "a", "base")
	s := fixture(t, root, nil)
	put(t, root, "a", "next")
	batch := resolve(t, s, "a")
	batch.Upserts[0].After.Hash = "tampered"
	view := s.View()
	view.Changes[0].Before.Meta.Path = "elsewhere"
	if got := s.View().Changes[0]; got.After.Hash == "tampered" || got.Before.Meta.Path != "a" {
		t.Fatal("caller mutated store")
	}
	put(t, root, "a", "unresolved")
	if _, err := s.GetDiff(context.Background(), "a"); !errors.Is(err, ErrStale) {
		t.Fatalf("returned unversioned current content %v", err)
	}
	before := s.View()
	if _, err := s.Reset(context.Background(), []string{"a", "missing"}); err == nil {
		t.Fatal("partial reset")
	}
	if !reflect.DeepEqual(before, s.View()) {
		t.Fatal("failed reset cleared changes")
	}
	batch, err := s.Reset(context.Background(), []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Reload || batch.Generation != 2 || len(s.View().Changes) != 0 {
		t.Fatal(batch)
	}
	put(t, root, "a", "newest")
	resolve(t, s, "a")
	r, err := s.GetDiff(context.Background(), "a")
	if err != nil || r.Generation != 2 || !strings.Contains(diff.Unified(r), "-unresolved") {
		t.Fatalf("reset before %+v %v", r, err)
	}
	if _, err := s.GetDiff(context.Background(), "../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := s.Resolve(context.Background(), "../escape"); err == nil {
		t.Fatal("resolver traversal accepted")
	}
}

func TestEntryCapacityAndCancellation(t *testing.T) {
	root := t.TempDir()
	s := fixture(t, root, func(o *Options) { o.MaxEntries = 2 })
	put(t, root, "a", "a")
	put(t, root, "b", "b")
	resolve(t, s, "a")
	resolve(t, s, "b")
	put(t, root, "c", "c")
	if _, err := s.Resolve(context.Background(), "c"); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	if len(s.View().Changes) != 2 {
		t.Fatal("partial over-cap commit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.ResolveBatch(ctx, nil, true); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
