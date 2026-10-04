package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type testValue struct {
	Number int
	Text   string
}

func TestCatalogPagesCloneIsolationCancellationAndCleanup(t *testing.T) {
	dir := t.TempDir()
	s, err := New[testValue](dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for start := 0; start < 513; start += PageSize {
		var entries []Entry[testValue]
		for i := start; i < start+PageSize && i < 513; i++ {
			entries = append(entries, Entry[testValue]{Key: fmt.Sprintf("scope/%04d", i), Value: testValue{i, "中文"}})
		}
		if err := s.Apply(entries); err != nil {
			t.Fatal(err)
		}
	}
	if s.Len() != 513 {
		t.Fatal(s.Len())
	}
	page, count, err := s.Window(context.Background(), 500, 100, nil)
	if err != nil || count != 513 || len(page) != 13 || page[0].Value.Number != 500 {
		t.Fatalf("page=%v count=%d err=%v", page, count, err)
	}
	page[0].Value.Text = "not owned by catalog"
	fresh, ok, err := s.Get("scope/0500")
	if err != nil || !ok || fresh.Text != "中文" {
		t.Fatal(fresh, ok, err)
	}
	filtered, count, err := s.Window(context.Background(), 0, 10, func(key string) bool { return strings.HasSuffix(key, "0512") })
	if err != nil || count != 1 || filtered[0].Value.Number != 512 {
		t.Fatal(filtered, count, err)
	}
	clone, err := s.Clone(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.Delete("scope/0512"); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 513 || clone.Len() != 512 {
		t.Fatal("clone mutated source")
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Walk(ctx, "", func(string, testValue) error { t.Fatal("callback after cancel"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.Apply(make([]Entry[testValue], PageSize+1)); err == nil {
		t.Fatal("unbounded transaction admitted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("scope/0000"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("catalog files leaked", files, err)
	}
}

func TestWalkReleasesReadTransactionBeforeCallbackWrites(t *testing.T) {
	s, err := New[testValue](t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 300; i++ {
		if err := s.Put(fmt.Sprint(i+1000), testValue{i, "old"}); err != nil {
			t.Fatal(err)
		}
	}
	visited := 0
	if err := s.Walk(context.Background(), "", func(key string, value testValue) error {
		visited++
		value.Text = strings.Repeat("new", 100)
		return s.Put(key, value)
	}); err != nil {
		t.Fatal(err)
	}
	if visited != 300 {
		t.Fatal(visited)
	}
}
