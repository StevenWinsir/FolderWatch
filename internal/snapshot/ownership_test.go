package snapshot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCapturedBufferOwnershipIsIndependent(t *testing.T) {
	root := t.TempDir()
	opts := Options{MaxFileBytes: 8 << 20, MemoryFileBytes: 64 << 10, MemoryBytes: 1 << 20, MaxFiles: 10}
	store, err := New(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	want := bytes.Repeat([]byte("original\n"), 1000)
	path := filepath.Join(root, "a")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := store.Capture(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := store.Capture(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	read, err := store.ReadContent(ctx, first)
	if err != nil || !bytes.Equal(read, want) {
		t.Fatalf("capture reused backing bytes: %v", err)
	}
	read[0] = '!'
	again, err := store.ReadContent(ctx, first)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatalf("caller mutated owned bytes: %v", err)
	}
	if err := store.Delete(second); err != nil {
		t.Fatal(err)
	}
	again, err = store.ReadContent(ctx, first)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatalf("independent deletion changed snapshot: %v", err)
	}
}
