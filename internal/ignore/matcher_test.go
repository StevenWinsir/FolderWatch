package ignore

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func put(t *testing.T, root, key, text string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRuleMatrix(t *testing.T) {
	cases := []struct {
		name         string
		rules        []string
		path         string
		dir, ignored bool
	}{
		{"basename", []string{"*.tmp"}, "deep/file.tmp", false, true},
		{"root anchor", []string{"/cache"}, "nested/cache", true, false},
		{"root match", []string{"/cache"}, "cache", true, true},
		{"slash anchor", []string{"src/*.go"}, "nested/src/a.go", false, false},
		{"slash match", []string{"src/*.go"}, "src/a.go", false, true},
		{"directory", []string{"cache/"}, "cache/a", false, true},
		{"not regular file", []string{"cache/"}, "cache", false, false},
		{"double star zero", []string{"a/**/b"}, "a/b", false, true},
		{"double star deep", []string{"a/**/b"}, "a/x/y/b", false, true},
		{"recursive does not exclude parent", []string{"a/**"}, "a", true, false},
		{"recursive child", []string{"a/**"}, "a/b", false, true},
		{"negation", []string{"*.log", "!keep.log"}, "keep.log", false, false},
		{"excluded parent", []string{"cache/", "!cache/keep"}, "cache/keep", false, true},
		{"unignore parent", []string{"cache/", "!cache/", "!cache/keep"}, "cache/keep", false, false},
		{"child negation under recursive", []string{"a/**", "!a/b"}, "a/b", false, false},
		{"comment", []string{"#notes"}, "#notes", false, false},
		{"escaped comment", []string{`\#notes`}, "#notes", false, true},
		{"escaped bang", []string{`\!important`}, "!important", false, true},
		{"trim spaces", []string{"name   "}, "name", false, true},
		{"literal space", []string{`name\ `}, "name ", false, true},
		{"CRLF", []string{"*.tmp\r"}, "file.tmp", false, true},
		{"BOM", []string{"\ufeff*.tmp"}, "file.tmp", false, true},
		{"question", []string{"file?.txt"}, "file1.txt", false, true},
		{"character class", []string{"[ab].txt"}, "b.txt", false, true},
		{"literal braces", []string{"literal{a,b}"}, "literal{a,b}", false, true},
		{"no brace expansion", []string{"literal{a,b}"}, "literala", false, false},
		{"unicode", []string{"目录/*.临时"}, "目录/文件.临时", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(Options{Root: t.TempDir(), Patterns: tc.rules})
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.Match(tc.path, tc.dir)
			if err != nil || got != tc.ignored {
				t.Fatalf("Match(%q,%v)=%v,%v want %v", tc.path, tc.dir, got, err, tc.ignored)
			}
		})
	}
}

func TestDefaultsAndSources(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".gitignore", "*.log\nblocked/\n")
	put(t, root, "nested/.gitignore", "!keep.log\nlocal.tmp\n/root-only\n")
	put(t, root, ".folderwatchignore", "!global.log\nnested/force.txt\n")
	extra := filepath.Join(t.TempDir(), "extra")
	if err := os.WriteFile(extra, []byte("!nested/force.txt\n*.cli\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Root: root, IgnoreFile: extra, RespectGitIgnore: true, Patterns: []string{"!keep.cli"}})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		".git/HEAD": true, "nested/.git/config": true, "other.log": true, "global.log": false,
		"nested/keep.log": false, "nested/other.log": true, "nested/local.tmp": true,
		"nested/root-only": true, "nested/deep/root-only": false, "else/local.tmp": false,
		"nested/force.txt": false, "drop.cli": true, "keep.cli": false, "blocked/file": true,
	} {
		got, err := m.Match(key, false)
		if err != nil || got != want {
			t.Errorf("%q=%v %v want %v", key, got, err, want)
		}
	}
	plain, err := New(Options{Root: root, IncludeGit: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{".git/HEAD", "other.log", "nested/local.tmp"} {
		if got, err := plain.Match(key, false); err != nil || got {
			t.Fatalf("disabled Git rules: %q %v %v", key, got, err)
		}
	}
	put(t, root, "late/.gitignore", "*.fresh\n")
	if got, err := m.Match("late/file.fresh", false); err != nil || !got {
		t.Fatalf("new runtime directory: %v %v", got, err)
	}
}

func TestRuleCacheAndConcurrency(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".gitignore", "*.old\n")
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, ".gitignore", "*.new\n")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if got, err := m.Match("future/a.old", false); err != nil || !got {
					t.Errorf("cached match: %v %v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	reloaded, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reloaded.Match("a.new", false); err != nil || !got {
		t.Fatalf("reload: %v %v", got, err)
	}
}

func TestMalformedRulesAndDirectoryPreflight(t *testing.T) {
	for _, pattern := range []string{"[", "trailing\\", "bad\x00pattern"} {
		if _, err := New(Options{Root: t.TempDir(), Patterns: []string{pattern}}); err == nil {
			t.Fatalf("accepted %q", pattern)
		}
	}
	root := t.TempDir()
	put(t, root, "bad/.gitignore", "[invalid\n")
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Match("bad", true); err == nil {
		t.Fatal("directory with broken rules must be rejected before descent")
	}
	if _, err := m.Match("../escape", false); err == nil {
		t.Fatal("accepted outside-root path")
	}
	if _, err := New(Options{Root: root, IgnoreFile: filepath.Join(root, "missing")}); err == nil {
		t.Fatal("missing explicit ignore file accepted")
	}
}

func TestSymlinkBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privilege varies")
	}
	root, outside := t.TempDir(), t.TempDir()
	put(t, outside, ".gitignore", "*\n")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Match("link/secret", false); err == nil {
		t.Fatal("followed link ancestor")
	}
	if _, err := m.Match("link", true); err == nil {
		t.Fatal("trusted incorrect directory hint for symlink")
	}
	if got, err := m.Match("link", false); err != nil || got {
		t.Fatalf("link metadata: %v %v", got, err)
	}
	if err := os.Symlink(filepath.Join(outside, ".gitignore"), filepath.Join(root, ".folderwatchignore")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{Root: root}); err == nil {
		t.Fatal("followed automatic ignore symlink")
	}
}
