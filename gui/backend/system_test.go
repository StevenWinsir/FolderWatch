package backend

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommandPartsKeepsQuotedArgumentsWithoutShell(t *testing.T) {
	got, err := commandParts(`code --reuse-window "Project Folder"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--reuse-window", "Project Folder"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	for _, input := range []string{"code; touch /tmp/pwned", `code "unterminated`} {
		if _, err := commandParts(input); err == nil {
			t.Fatalf("accepted unsafe command %q", input)
		}
	}
}

func TestSystemActionsValidateRootRelativePaths(t *testing.T) {
	_, a, client := testFacade(t)
	root := t.TempDir()
	key := filepath.Join("dir", "file '世界'.txt")
	if err := ensureDirAndWrite(root, key); err != nil {
		t.Fatal(err)
	}
	record := []string{}
	a.setSystemActions(systemActions{
		openEditor: func(_ context.Context, editor, path string) error {
			record = append(record, "open:"+editor+":"+path)
			return nil
		},
		reveal: func(_ context.Context, path string) error { record = append(record, "reveal:"+path); return nil },
		copy:   func(_ context.Context, path string) error { record = append(record, "copy:"+path); return nil },
	})
	req := started(t, a, client, root)
	if reply := a.OpenInEditor(EditorRequest{PathRequest: PathRequest{SessionRequest: req, Path: key}, Editor: `code --reuse-window`}); reply.Error != nil {
		t.Fatal(reply)
	}
	if reply := a.RevealInFinder(PathRequest{SessionRequest: req, Path: key}); reply.Error != nil {
		t.Fatal(reply)
	}
	if reply := a.CopyPath(CopyPathRequest{PathRequest: PathRequest{SessionRequest: req, Path: key}, Relative: true}); reply.Error != nil {
		t.Fatal(reply)
	}
	if _, _, err := a.pathFor(req, "../outside"); err == nil {
		t.Fatal("accepted traversal")
	}
	if !strings.Contains(strings.Join(record, "\n"), key) || len(record) != 3 {
		t.Fatalf("unexpected actions: %v", record)
	}
}

func TestGetSettingsUsesSharedProjectConfig(t *testing.T) {
	_, a, client := testFacade(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".folderwatch.toml"), []byte("debounce = \"25ms\"\nmax_diff_bytes = \"2MiB\"\nrespect_gitignore = true\neditor = \"code --reuse-window\"\nignore = [\"*.tmp\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := a.GetSettings(SettingsRequest{ClientID: client, Root: root})
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if reply.Settings.Debounce != "25ms" || reply.Settings.MaxDiffBytes != "2097152" || !reply.Settings.RespectGitIgnore || reply.Settings.Editor != "code --reuse-window" || len(reply.Settings.Ignore) != 1 {
		t.Fatalf("unexpected shared settings: %+v", reply.Settings)
	}
}

func ensureDirAndWrite(root, key string) error {
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("before\n"), 0600)
}
