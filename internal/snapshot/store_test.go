package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func options(t *testing.T) Options {
	t.Helper()
	return Options{MaxFileBytes: 1 << 20, MemoryFileBytes: 64, MemoryBytes: 128, DiskBytes: 2 << 20, MaxFiles: 100, TempDir: t.TempDir()}
}
func newStore(t *testing.T, root string, opts Options) *Store {
	t.Helper()
	s, err := New(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func writeFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureStorageBudgetsAndHashes(t *testing.T) {
	root := t.TempDir()
	opts := options(t)
	opts.MaxFileBytes = 512
	opts.DiskBytes = 250
	s := newStore(t, root, opts)
	cases := []struct {
		name      string
		data      []byte
		retention string
	}{
		{"small", []byte("hello"), "memory"},
		{"unicode", []byte("你好\n"), "memory"},
		{"medium", bytes.Repeat([]byte("x"), 200), "disk"},
		{"budget", bytes.Repeat([]byte("y"), 200), "budget"},
		{"huge", bytes.Repeat([]byte("z"), 513), "size"},
		{"binary", []byte{'x', 0, 'y'}, "binary"},
		{"invalid-utf8", []byte{0xff, 0x80}, "binary"},
		{"empty", []byte{}, "memory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, root, tc.name, tc.data)
			ref, err := s.Capture(context.Background(), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(tc.data)
			if ref.Retention != tc.retention || ref.Hash != hex.EncodeToString(sum[:]) {
				t.Fatalf("ref=%+v", ref)
			}
			data, err := s.ReadContent(context.Background(), ref)
			if tc.retention == "memory" || tc.retention == "disk" {
				if err != nil || !bytes.Equal(data, tc.data) {
					t.Fatalf("content %q %v", data, err)
				}
			} else if !errors.Is(err, ErrNoContent) {
				t.Fatalf("wanted metadata-only, got %v", err)
			}
		})
	}
	stats := s.Stats()
	if stats.MemoryBytes > opts.MemoryBytes || stats.DiskBytes > opts.DiskBytes {
		t.Fatalf("budget overflow %+v", stats)
	}
	info, err := os.Stat(s.dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("cache permissions %v %v", info, err)
	}
	for _, item := range s.items {
		if item.file != "" {
			info, err := os.Stat(item.file)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("file permissions %v %v", info, err)
			}
		}
	}
}

func TestBaselineStableResetRollbackAndReferenceOwnership(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, root, "a", []byte("hello"))
	if err := s.Reset(ctx, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	first := s.Baseline()
	ref := first.Files["a"]
	for _, content := range []string{"hello1", "hello2", "hello"} {
		writeFile(t, root, "a", []byte(content))
		current, err := s.Capture(ctx, "a")
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.ReadContent(ctx, ref)
		if err != nil || string(got) != "hello" {
			t.Fatalf("baseline advanced %q %v", got, err)
		}
		if (current.Hash == ref.Hash) != (content == "hello") {
			t.Fatal("hash restoration differs")
		}
		if err := s.Delete(current); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(ref); err == nil {
		t.Fatal("deleted a pinned baseline")
	}
	writeFile(t, root, "a", []byte("reset version"))
	if err := s.Reset(ctx, []string{"a", "missing"}); err == nil {
		t.Fatal("accepted partial reset")
	}
	if s.Baseline().Generation != 1 {
		t.Fatal("failed reset published")
	}
	got, err := s.ReadContent(ctx, ref)
	if err != nil || string(got) != "hello" {
		t.Fatal("rollback lost baseline")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Reset(cancelled, []string{"a"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	// Returned maps and content bytes are copies, not shared mutable state.
	delete(first.Files, "a")
	got[0] = 'X'
	if len(s.Baseline().Files) != 1 {
		t.Fatal("caller mutated baseline map")
	}
	if err := s.Reset(ctx, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	second := s.Baseline()
	if second.Generation != 2 {
		t.Fatal("generation not advanced")
	}
	if _, err := s.ReadContent(ctx, ref); !errors.Is(err, ErrStale) {
		t.Fatalf("old ref %v", err)
	}
	got, err = s.ReadContent(ctx, second.Files["a"])
	if err != nil || string(got) != "reset version" {
		t.Fatalf("new baseline %q %v", got, err)
	}
	forged := second.Files["a"]
	forged.Meta.Size++
	if _, err := s.ReadContent(ctx, forged); !errors.Is(err, ErrStale) {
		t.Fatal("accepted forged ref")
	}
	other := newStore(t, root, options(t))
	if _, err := other.ReadContent(ctx, second.Files["a"]); !errors.Is(err, ErrStale) {
		t.Fatal("accepted foreign ref")
	}
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("cache survived close")
	}
	if _, err := s.Capture(ctx, "a"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed capture %v", err)
	}
}

func TestSnapshotPathAndEntryLimits(t *testing.T) {
	root := t.TempDir()
	opts := options(t)
	opts.MaxFiles = 1
	s := newStore(t, root, opts)
	writeFile(t, root, "a", []byte("a"))
	if _, err := s.Capture(context.Background(), "../escape"); err == nil {
		t.Fatal("accepted traversal")
	}
	ref, err := s.Capture(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Capture(context.Background(), "a"); err == nil {
		t.Fatal("unbounded references")
	}
	if err := s.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Capture(context.Background(), "a"); err != nil {
		t.Fatal("budget not released")
	}
	opts.TempDir = root
	if bad, err := New(root, opts); err == nil {
		bad.Close()
		t.Fatal("cache inside root")
	}
	opts = options(t)
	opts.MaxFileBytes = 0
	if _, err := New(root, opts); err == nil {
		t.Fatal("accepted invalid options")
	}
}

func TestFullStreamUTF8StorageProbe(t *testing.T) {
	p := textProbe{}
	data := []byte("a你好z")
	for _, b := range data {
		p.write([]byte{b})
	}
	if p.binary || len(p.tail) != 0 {
		t.Fatal("split UTF8 rejected")
	}
	p.write([]byte{0xe4})
	if len(p.tail) != 1 {
		t.Fatal("missing incomplete tail")
	}
	p.write([]byte{0xff})
	if !p.binary {
		t.Fatal("invalid continuation accepted")
	}
	root := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, root, "late-binary", append(bytes.Repeat([]byte("a"), 100000), 0))
	ref, err := s.Capture(context.Background(), "late-binary")
	if err != nil || ref.HasContent || ref.Retention != "binary" {
		t.Fatalf("late binary %+v %v", ref, err)
	}
	if s.Stats().DiskBytes != 0 {
		t.Fatal("binary spool retained")
	}
}

func TestLargeCaptureCancellationAndConcurrentClose(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, options(t))
	file, err := os.Create(filepath.Join(root, "sparse"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(2 << 30); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := s.Capture(ctx, "sparse"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("capture cancellation %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Capture(context.Background(), "sparse"); done <- err }()
	time.Sleep(5 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrClosed) {
			t.Fatalf("close capture %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("capture leaked after close")
	}
}

func TestConcurrentBaselineReadsAndResets(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, root, "a", []byte(strings.Repeat("x", 100)))
	if err := s.Reset(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b := s.Baseline()
				if ref, ok := b.Files["a"]; ok {
					_, err := s.ReadContent(context.Background(), ref)
					if err != nil && !errors.Is(err, ErrStale) {
						t.Error(err)
					}
				}
			}
		}()
	}
	for i := 0; i < 10; i++ {
		if err := s.Reset(context.Background(), []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if s.Stats().References != 1 {
		t.Fatal("reset leaked references")
	}
}
