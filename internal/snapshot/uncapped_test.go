package snapshot

import (
	"context"
	"fmt"
	"testing"
)

func TestUncappedReferenceCountRetainsByteBudgetsAndAtomicReset(t *testing.T) {
	root := t.TempDir()
	opts := options(t)
	opts.MaxFiles = 0
	opts.MemoryBytes = 8
	opts.DiskBytes = 8
	s := newStore(t, root, opts)
	paths := make([]string, 257)
	for i := range paths {
		paths[i] = fmt.Sprintf("f-%03d", i)
		writeFile(t, root, paths[i], []byte("01234567"))
	}
	ctx := context.Background()
	if err := s.Reset(ctx, paths); err != nil {
		t.Fatal(err)
	}
	stats := s.Stats()
	if stats.References != len(paths) || stats.MemoryBytes > opts.MemoryBytes || stats.DiskBytes > opts.DiskBytes {
		t.Fatalf("budgets: %+v", stats)
	}
	last := s.Baseline().Files[paths[len(paths)-1]]
	if last.Hash == "" || last.HasContent || last.Retention != "budget" {
		t.Fatalf("uncapped metadata lost bounded retention: %+v", last)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Reset(cancelled, paths); err == nil {
		t.Fatal("cancelled reset committed")
	}
	if s.Baseline().Generation != 1 || len(s.Baseline().Files) != len(paths) {
		t.Fatal("cancelled reset damaged baseline")
	}
	opts.MaxFiles = -1
	if _, err := New(root, opts); err == nil {
		t.Fatal("accepted negative reference cap")
	}
}
