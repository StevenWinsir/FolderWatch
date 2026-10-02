package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/testutil"
)

func TestBurstOverflowConvergesWithoutConsumer(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 100; i++ {
		putSession(t, root, fmt.Sprintf("f-%03d", i), "baseline")
	}
	delay, pending := "10ms", 8
	prepared, err := Prepare(context.Background(), root, config.Overlay{Debounce: &delay}, config.LoadOptions{SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	prepared.Config.MaxPendingEvents = pending
	s, err := StartSession(context.Background(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Do not consume Events. Overflow must still converge through the core.
	const content = "final burst content"
	for round := 0; round < 3; round++ {
		for i := 0; i < 100; i++ {
			putSession(t, root, fmt.Sprintf("f-%03d", i), content)
		}
	}
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])
	deadline := time.Now().Add(10 * time.Second)
	for {
		view := s.ChangeState()
		ok := len(view.Changes) == 100
		for _, c := range view.Changes {
			ok = ok && c.Kind == changes.Modified && c.After != nil && c.After.Hash == want
		}
		if ok {
			break
		}
		if s.Err() != nil {
			t.Fatal(s.Err())
		}
		if time.Now().After(deadline) {
			t.Fatalf("overflow lost final state: %d changes", len(view.Changes))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.Baseline().Generation != 1 {
		t.Fatal("burst advanced baseline")
	}
	if len(s.events) > 32 || cap(s.events) != 32 {
		t.Fatal("unbounded application queue")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedLifecycleReleasesDescriptorsAndCaches(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	putSession(t, root, "a", "baseline")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, cache)
	}
	beforeG, beforeFD := runtime.NumGoroutine(), resourceFDCount()
	for cycle := 0; cycle < 30; cycle++ {
		prepared, err := Prepare(context.Background(), root, config.Overlay{}, config.LoadOptions{SkipUserConfig: true})
		if err != nil {
			t.Fatal(err)
		}
		s, err := StartSession(context.Background(), prepared)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Pause(context.Background()); err != nil {
			_ = s.Close()
			t.Fatal(err)
		}
		if err := s.ResetBaseline(context.Background()); err != nil {
			_ = s.Close()
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > beforeG+2 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > beforeG+2 {
		t.Fatalf("goroutines %d -> %d", beforeG, after)
	}
	if after := resourceFDCount(); beforeFD >= 0 && after > beforeFD+2 {
		t.Fatalf("descriptors %d -> %d", beforeFD, after)
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache entries leaked: %v", entries)
	}
}

func resourceFDCount() int {
	return testutil.OpenDescriptors()
}
