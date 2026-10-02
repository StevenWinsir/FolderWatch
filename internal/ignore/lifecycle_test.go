package ignore

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryIdentityReloadButExistingRulesStayFrozen(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scope")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/old", false); err != nil || !ignored {
		t.Fatalf("old rule %v %v", ignored, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/old", false); err != nil || !ignored {
		t.Fatal("existing scope unexpectedly hot reloaded")
	}
	// Keep the retired inode alive to make identity distinction deterministic.
	retired := filepath.Join(t.TempDir(), "retired")
	if err := os.Rename(dir, retired); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/new", false); err != nil || !ignored {
		t.Fatalf("replacement scope %v %v", ignored, err)
	}
	if ignored, err := m.Match("scope/old", false); err != nil || ignored {
		t.Fatal("retained prior directory rules")
	}
}

func TestRemovedScopeCacheIsRetired(t *testing.T) {
	root := t.TempDir()
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("directory-%d", i)
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, "deep"), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Match(name+"/deep/file", false); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		m.ForgetDirectory(name)
		// Late deleted-path notifications must not recreate a nonexistent scope.
		if _, err := m.Match(name+"/deep/file", false); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.git) != 1 {
		t.Fatalf("retired scopes accumulated: %d", len(m.git))
	}
	m.ForgetDirectory("../outside")
	m.ForgetDirectory(".")
	if len(m.git) != 1 {
		t.Fatal("invalid retirement removed root policy")
	}
}
