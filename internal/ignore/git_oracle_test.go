package ignore

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compare the supported rule subset to the installed Git, in an isolated temp
// repository with global/system excludes disabled. FolderWatch itself uses no Git.
func TestGitIgnoreOracle(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git oracle requires git")
	}
	root := t.TempDir()
	run := func(args []string, input string) ([]byte, error) {
		cmd := exec.Command(git, append([]string{"-c", "core.excludesFile=", "-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		cmd.Stdin = strings.NewReader(input)
		return cmd.Output()
	}
	if _, err := run([]string{"init", "-q"}, ""); err != nil {
		t.Fatal(err)
	}
	files := []string{"root.log", "nested/child.log", "keep.log", "cache/item", "nested/cache/item", "a/b.txt", "a/x/b.txt", "a/file", "literal{a,b}", "literala", "#hash", "!bang", "space ", "simple", "nested/rootonly", "rootonly", "file1.txt", "b.txt"}
	for _, key := range files {
		put(t, root, key, "")
	}
	paths := append(append([]string{}, files...), "a", "cache", "nested/cache", "nested")
	ruleSets := []string{
		"*.log\n!keep.log\n", "/rootonly\n", "cache/\n!cache/item\n",
		"cache/\n!cache/\n!cache/item\n", "a/**\n!a/b.txt\n", "a/**/b.txt\n",
		"**/cache/**\n", "nested/*\n!nested/cache/\n!nested/cache/item\n",
		"/a/*\n!/a/b.txt\n", "\\#hash\n\\!bang\nspace\\ \n",
		"literal{a,b}\n", "[ab].txt\nfile?.txt\n", "simple   \r\n",
		"*\n!a/\n!a/b.txt\n", "**/*.log\n", "nested/\n!nested/cache/item\n",
	}
	for _, rules := range ruleSets {
		t.Run(strings.ReplaceAll(strings.TrimSpace(rules), "\n", ";"), func(t *testing.T) {
			put(t, root, ".gitignore", rules)
			output, err := run([]string{"check-ignore", "--no-index", "-z", "--stdin"}, strings.Join(paths, "\x00")+"\x00")
			if err != nil {
				if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
					t.Fatal(err)
				}
			}
			want := make(map[string]bool)
			for _, item := range bytes.Split(output, []byte{0}) {
				if len(item) > 0 {
					want[string(item)] = true
				}
			}
			m, err := New(Options{Root: root, RespectGitIgnore: true, IncludeGit: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range paths {
				info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(key)))
				if err != nil {
					t.Fatal(err)
				}
				got, err := m.Match(key, info.IsDir())
				if err != nil || got != want[key] {
					t.Errorf("%q: matcher=%v err=%v Git=%v", key, got, err, want[key])
				}
			}
		})
	}
}
