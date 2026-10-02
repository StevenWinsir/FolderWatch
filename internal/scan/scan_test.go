package scan

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/model"
)

func fixture(t *testing.T, root, key string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0600); err != nil {
		t.Fatal(err)
	}
}
func matcher(t *testing.T, root string, respect bool) *ignore.Matcher {
	t.Helper()
	m, err := ignore.New(ignore.Options{Root: root, RespectGitIgnore: respect})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func entries(result Result) map[string]model.FileMeta {
	m := make(map[string]model.FileMeta)
	for _, e := range result.Entries {
		m[e.Path] = e
	}
	return m
}

func TestNestedUnicodeBinaryAndIgnore(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, ".folderwatchignore", []byte("ignored/\n*.tmp\n"))
	for _, key := range []string{"目录 with spaces/a.xyz", "nested/deep/file", ".hidden", ".git/HEAD", "ignored/file", "nested/drop.tmp"} {
		fixture(t, root, key, []byte{0, 1, 2, 255})
	}
	m := matcher(t, root, false)
	result, err := Scan(context.Background(), root, m)
	if err != nil || len(result.Warnings) != 0 {
		t.Fatalf("scan: %v %+v", err, result.Warnings)
	}
	got := entries(result)
	for _, key := range []string{".", ".folderwatchignore", "目录 with spaces", "目录 with spaces/a.xyz", "nested/deep/file", ".hidden"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing %q", key)
		}
	}
	for _, key := range []string{".git", ".git/HEAD", "ignored", "ignored/file", "nested/drop.tmp"} {
		if _, ok := got[key]; ok {
			t.Errorf("included ignored %q", key)
		}
	}
	if got["目录 with spaces/a.xyz"].Size != 4 || got["目录 with spaces/a.xyz"].Kind != model.RegularFile {
		t.Fatal("metadata was not preserved")
	}
	again, err := Scan(context.Background(), root, m)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatalf("non-deterministic scan: %v", err)
	}
}

func TestSymlinksAndCycles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege varies")
	}
	root, outside := t.TempDir(), t.TempDir()
	fixture(t, outside, "secret", []byte("outside"))
	for name, target := range map[string]string{"loop": root, "external": outside, "broken": filepath.Join(root, "missing")} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Scan(context.Background(), root, matcher(t, root, false))
	if err != nil || len(result.Entries) != 4 {
		t.Fatalf("symlinks traversed: %+v %v", result, err)
	}
	for _, e := range result.Entries {
		if e.Path != "." && e.Kind != model.Symlink {
			t.Fatalf("not symlink metadata: %+v", e)
		}
	}
}

func TestInitialAndRuntimeUseSameMatcher(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, ".folderwatchignore", []byte("*.tmp\ncache/\n"))
	fixture(t, root, "old/keep", nil)
	m := matcher(t, root, true)
	first, err := Scan(context.Background(), root, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range first.Entries {
		if got, err := m.Match(e.Path, e.Kind == model.Directory); err != nil || got {
			t.Fatalf("initial/runtime mismatch: %+v %v", e, err)
		}
	}
	fixture(t, root, "late/.gitignore", []byte("*.local\n"))
	candidates := []string{"late/keep", "late/drop.tmp", "late/drop.local", "cache/keep"}
	for _, key := range candidates {
		fixture(t, root, key, nil)
	}
	second, err := Scan(context.Background(), root, m)
	if err != nil {
		t.Fatal(err)
	}
	got := entries(second)
	for _, key := range candidates {
		ignored, err := m.Match(key, false)
		_, included := got[key]
		if err != nil || included == ignored {
			t.Fatalf("runtime mismatch %q: included=%v ignored=%v err=%v", key, included, ignored, err)
		}
	}
}

type filterFunc func(string, bool) (bool, error)

func (f filterFunc) Match(p string, d bool) (bool, error) { return f(p, d) }

func TestCancellationAndRootFailure(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "a", nil)
	fixture(t, root, "b", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, root, matcher(t, root, false)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	filter := filterFunc(func(p string, d bool) (bool, error) { cancel(); return false, nil })
	if _, err := Scan(ctx, root, filter); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-scan cancellation: %v", err)
	}
	if _, err := Scan(context.Background(), filepath.Join(root, "missing"), filter); err == nil {
		t.Fatal("missing root accepted")
	}
	if _, err := Scan(context.Background(), root, nil); err == nil {
		t.Fatal("nil matcher accepted")
	}
}

func TestRecoverableErrorsAreWarnings(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "bad/sub/file", nil)
	fixture(t, root, "good", nil)
	fixture(t, root, "vanished", nil)
	walk := func(root string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if filepath.Base(p) == "bad" {
				return fn(p, d, fs.ErrPermission)
			}
			if filepath.Base(p) == "vanished" {
				return fn(p, d, fs.ErrNotExist)
			}
			return fn(p, d, err)
		})
	}
	result, err := scanWithWalker(context.Background(), root, matcher(t, root, false), walk)
	if err != nil || len(result.Warnings) != 2 {
		t.Fatalf("warnings: %+v %v", result, err)
	}
	if _, ok := entries(result)["good"]; !ok {
		t.Fatal("lost readable sibling")
	}
	if _, ok := entries(result)["bad"]; ok {
		t.Fatal("unreadable directory offered for watching")
	}
	fatal := func(root string, fn fs.WalkDirFunc) error { return fn(root, nil, fs.ErrPermission) }
	if _, err := scanWithWalker(context.Background(), root, matcher(t, root, false), fatal); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("root error: %v", err)
	}
}

func TestMalformedNestedIgnoreSkipsSubtree(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "broken/.gitignore", []byte("[\n"))
	fixture(t, root, "broken/file", nil)
	fixture(t, root, "good", nil)
	result, err := Scan(context.Background(), root, matcher(t, root, true))
	if err != nil || len(result.Warnings) != 1 || result.Warnings[0].Path != "broken" {
		t.Fatalf("expected one subtree warning: %+v %v", result, err)
	}
	if _, ok := entries(result)["broken"]; ok {
		t.Fatal("broken subtree registered")
	}
	if _, ok := entries(result)["good"]; !ok {
		t.Fatal("good sibling lost")
	}
}
