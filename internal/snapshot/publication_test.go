package snapshot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelledWhileWaitingForPublicationKeepsOldBaseline(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, root, "a", []byte("before"))
	if err := s.Reset(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "a", []byte("after"))
	s.mu.RLock()
	held := true
	defer func() {
		if held {
			s.mu.RUnlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Reset(ctx, []string{"a"}) }()
	// A pending writer makes TryRLock fail. With our initial RLock retained,
	// Reset has finished staging and is waiting exactly at publication, not I/O.
	deadline := time.Now().Add(3 * time.Second)
	for s.mu.TryRLock() {
		s.mu.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("reset did not reach publication")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	s.mu.RUnlock()
	held = false
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled reset committed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reset remained blocked")
	}
	if s.Baseline().Generation != 1 {
		t.Fatal("publication advanced after cancellation")
	}
	got, err := s.ReadContent(context.Background(), s.Baseline().Files["a"])
	if err != nil || string(got) != "before" {
		t.Fatalf("before content %q %v", got, err)
	}
}
