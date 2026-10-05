package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/model"
)

func TestStreamMatchesInventoryAndCleansQueue(t *testing.T) {
	root, temp, outside := t.TempDir(), t.TempDir(), t.TempDir()
	for i := 0; i < 300; i++ {
		path := filepath.Join(root, fmt.Sprintf("d-%03d", i), "中文", "file")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("baseline"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "private"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	matcher, err := ignore.New(ignore.Options{Root: root, Patterns: []string{"d-000/"}})
	if err != nil {
		t.Fatal(err)
	}
	root = matcher.Root()
	want, err := Scan(context.Background(), root, matcher)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.EntryKind{}
	if err := Stream(context.Background(), root, ".", temp, matcher, func(meta model.FileMeta, err error) error {
		if err != nil {
			return err
		}
		got[meta.Path] = meta.Kind
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expected := map[string]model.EntryKind{}
	for _, entry := range want.Entries {
		expected[entry.Path] = entry.Kind
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatal("stream did not preserve inclusion and symlink policy")
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err = Stream(ctx, root, ".", temp, matcher, func(model.FileMeta, error) error { calls++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal(calls, err)
	}
	files, err := os.ReadDir(temp)
	if err != nil || len(files) != 0 {
		t.Fatal("walk queue leaked", files, err)
	}
}
