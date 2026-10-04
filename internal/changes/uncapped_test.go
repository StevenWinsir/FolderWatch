package changes

import (
	"context"
	"fmt"
	"testing"
)

func TestUncappedInventoryResolvesReconcilesAndClears(t *testing.T) {
	root := t.TempDir()
	s := fixture(t, root, func(o *Options) { o.MaxEntries = 0; o.Snapshot.MaxFiles = 0 })
	paths := make([]string, 257)
	for i := range paths {
		paths[i] = fmt.Sprintf("f-%03d", i)
		put(t, root, paths[i], "new")
	}
	batch, warnings, err := s.ResolveBatch(context.Background(), paths, false)
	if err != nil || len(warnings) > 0 || !batch.Reload || len(batch.Upserts) != 0 {
		t.Fatalf("resolve: upserts=%d warnings=%v err=%v", len(batch.Upserts), warnings, err)
	}
	if len(s.View().Changes) != len(paths) {
		t.Fatal("inventory was truncated")
	}
	reconcile(t, s)
	if len(s.View().Changes) != len(paths) {
		t.Fatal("reconciliation lost uncapped entries")
	}
	if _, err := s.Reset(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	if len(s.View().Changes) != 0 {
		t.Fatal("reset left stale changes")
	}
}
