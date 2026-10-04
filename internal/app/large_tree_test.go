package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/testutil"
)

// Assert authoritative semantics, not merely a root invalidation or ready event.
func waitExactChange(t *testing.T, s *Session, path string, kind changes.Kind, content string, timeout time.Duration) {
	t.Helper()
	digest := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(digest[:])
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, item := range s.ChangeState().Changes {
			if item.Path == path && item.Kind == kind && (kind == changes.Deleted || item.After != nil && item.After.Hash == want) {
				return
			}
		}
		select {
		case <-s.Done():
			t.Fatalf("session stopped: %v", s.Err())
		case <-deadline.C:
			t.Fatalf("missing exact %s state for %s; got %+v", kind, path, s.Changes())
		case <-s.Events():
		case <-tick.C:
		}
	}
}

func TestLargeTreePastBothFormerLimits(t *testing.T) {
	if os.Getenv("FOLDERWATCH_LARGE_TESTS") != "1" {
		t.Skip("set FOLDERWATCH_LARGE_TESTS=1 for the 9000-directory / 100005-file integration fixture")
	}
	const directories, files = 9000, 100005
	root := t.TempDir()
	start := time.Now()
	for i := 0; i < directories; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d-%05d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	name := func(i int) string { return fmt.Sprintf("d-%05d/file-%06d.txt", i%directories, i) }
	for i := 0; i < files; i++ {
		putSession(t, root, name(i), "baseline\n")
	}
	t.Logf("fixture: directories=%d files=%d creation=%s", directories, files, time.Since(start))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	beforeFD := testutil.OpenDescriptors()
	start = time.Now()
	prepared, err := Prepare(ctx, root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartSession(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := s.SnapshotStats().References; got != directories+files+1 {
		t.Fatalf("incomplete baseline: %d", got)
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	t.Logf("ready: entries=%d elapsed=%s descriptors=%d->%d heap_alloc=%d", s.SnapshotStats().References, time.Since(start), beforeFD, testutil.OpenDescriptors(), memory.HeapAlloc)
	// Independent edit beyond both old boundaries; no manually requested rescan.
	last := name(files - 1)
	start = time.Now()
	putSession(t, root, last, "later edit\n")
	waitExactChange(t, s, last, changes.Modified, "later edit\n", 3*time.Minute)
	t.Logf("last-file edit observed in %s", time.Since(start))
	if err := s.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, last, "paused edit\n")
	if err := s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	waitExactChange(t, s, last, changes.Modified, "paused edit\n", 3*time.Minute)
	if err := s.ResetBaseline(ctx); err != nil {
		t.Fatal(err)
	}
	if len(s.Changes()) != 0 {
		t.Fatal("reset did not clear changes")
	}
	if err := os.MkdirAll(filepath.Join(root, "new", "中文 空格", "deep"), 0700); err != nil {
		t.Fatal(err)
	}
	putSession(t, root, "new/中文 空格/deep/late.txt", "new subtree\n")
	waitExactChange(t, s, "new/中文 空格/deep/late.txt", changes.Added, "new subtree\n", 3*time.Minute)
	if err := os.Remove(filepath.Join(root, last)); err != nil {
		t.Fatal(err)
	}
	waitExactChange(t, s, last, changes.Deleted, "", 3*time.Minute)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("closed: descriptors=%d; pause/resume/reset/new subtree/delete all passed", testutil.OpenDescriptors())
}
