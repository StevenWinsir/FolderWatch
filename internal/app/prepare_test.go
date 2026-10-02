package app

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/config"
)

func TestPrepareReadOnlyAndReusableInputs(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{"a.txt": "hello", "drop.tmp": "ignored", ".folderwatchignore": "*.tmp\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	log, editor := "debug.log", "unused-editor --wait"
	prepared, err := Prepare(context.Background(), root, config.Overlay{LogFile: &log, Editor: &editor}, config.LoadOptions{CWD: root, SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(prepared.Config.Root) || len(prepared.Inventory.Entries) != 3 {
		t.Fatalf("prepared=%+v", prepared)
	}
	if ignored, err := prepared.Matcher.Match("future.tmp", false); err != nil || !ignored {
		t.Fatalf("runtime filter: %v %v", ignored, err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := func(items []os.DirEntry) []string {
		var out []string
		for _, e := range items {
			out = append(out, e.Name())
		}
		return out
	}
	if !reflect.DeepEqual(names(before), names(after)) {
		t.Fatal("Prepare wrote files into root")
	}
	if _, err := os.Stat(prepared.Config.LogFile); !os.IsNotExist(err) {
		t.Fatal("reserved logging option created a file")
	}
}

func TestPrepareErrorClasses(t *testing.T) {
	root := t.TempDir()
	_, err := Prepare(context.Background(), filepath.Join(root, "missing"), config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	var input *InputError
	if !errors.As(err, &input) {
		t.Fatalf("input class: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestCoreDoesNotImportUIOrWatcherAdapters(t *testing.T) {
	for _, dir := range []string{"app", "config", "ignore", "model", "pathutil", "scan", "fileutil", "snapshot", "debounce", "eventnorm"} {
		err := filepath.WalkDir(filepath.Join("..", dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				name, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					return err
				}
				for _, forbidden := range []string{"bubbletea", "lipgloss", "wails", "fsnotify", "spf13/pflag"} {
					if strings.Contains(name, forbidden) {
						t.Errorf("core %s imports adapter %s", path, name)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
