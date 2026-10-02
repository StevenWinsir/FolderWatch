package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "目录 with spaces")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeRoot("目录 with spaces/../目录 with spaces", root)
	if err != nil || got != want {
		t.Fatalf("root=%q err=%v want=%q", got, err, want)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"missing", "file", "~otheruser/x", "bad\x00path"} {
		if _, err := NormalizeRoot(input, root); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestKeys(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ input, want string }{
		{".", "."}, {root, "."}, {"a/../目录 with spaces/f", "目录 with spaces/f"},
		{filepath.Join(root, "a", "b"), "a/b"},
	} {
		got, err := Key(root, tc.input)
		if err != nil || got != tc.want {
			t.Errorf("Key(%q)=%q,%v want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"../escape", filepath.Join(root+"-sibling", "x"), "", "x\x00y"} {
		if _, err := Key(root, input); err == nil {
			t.Errorf("accepted escape %q", input)
		}
	}
}

func TestSymlinkPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege varies on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := CheckParents(root, "link/file"); err == nil {
		t.Fatal("followed symlink ancestor")
	}
	if err := CheckParents(root, "link"); err != nil {
		t.Fatal("final link metadata must be permitted:", err)
	}
	if err := CheckParents(root, "future/sub/file"); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(outside)
	got, err := NormalizeRoot("link", root)
	if err != nil || got != want {
		t.Fatalf("explicit root link = %q %v", got, err)
	}
}

func TestResolveHome(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	for _, tc := range []struct{ input, want string }{{"~", home}, {"~/文件", filepath.Join(home, "文件")}} {
		got, err := Resolve(tc.input, t.TempDir())
		if err != nil || got != tc.want {
			t.Fatalf("%q: %q %v", tc.input, got, err)
		}
	}
}

func FuzzKeyContainment(f *testing.F) {
	for _, input := range []string{".", "a/b", "../escape", "目录", "a/../../b", "a\x00b"} {
		f.Add(input)
	}
	root := f.TempDir()
	f.Fuzz(func(t *testing.T, input string) {
		key, err := Key(root, input)
		if err != nil {
			return
		}
		again, err := Key(root, filepath.Join(root, filepath.FromSlash(key)))
		if err != nil || again != key {
			t.Fatalf("key not idempotent: %q -> %q (%v)", key, again, err)
		}
		if !filepath.IsLocal(filepath.FromSlash(key)) {
			t.Fatalf("non-local key %q", key)
		}
	})
}
