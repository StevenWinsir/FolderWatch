package ignore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPruneRetiresDeletedScopesWithoutHotReloadingLiveRules(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scope")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Root: root, RespectGitIgnore: true})
	if err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/a.old", false); err != nil || !ignored {
		t.Fatalf("initial rules: %v %v", ignored, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.PruneDirectories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/a.old", false); err != nil || !ignored {
		t.Fatal("pruning hot-reloaded existing scope")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := m.PruneDirectories(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, exists := m.git["scope"]; exists {
		t.Fatal("deleted scope retained")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if ignored, err := m.Match("scope/a.new", false); err != nil || !ignored {
		t.Fatal("replacement did not acquire new rules")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.PruneDirectories(ctx); err != context.Canceled {
		t.Fatal(err)
	}
}
