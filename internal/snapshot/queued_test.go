package snapshot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitingCaptureAndResetAreCancellable(t *testing.T) {
	root := t.TempDir()
	s := newStore(t, root, options(t))
	writeFile(t, root, "a", []byte("a"))
	if err := s.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.release()
	for _, reset := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		done := make(chan error, 1)
		go func() {
			if reset {
				done <- s.Reset(ctx, []string{"a"})
			} else {
				_, err := s.Capture(ctx, "a")
				done <- err
			}
		}()
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("queued operation %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("cancelled operation blocked waiting for owner")
		}
		cancel()
	}
}

func FuzzTextProbeChunkBoundaries(f *testing.F) {
	for _, s := range []string{"hello", "你好", "a\x00b", "\xff", "\xe4\xbd"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		whole := textProbe{}
		whole.write(data)
		split := textProbe{}
		for _, b := range data {
			split.write([]byte{b})
		}
		if whole.binary != split.binary || (!whole.binary && string(whole.tail) != string(split.tail)) {
			t.Fatal("probe depends on chunk boundaries")
		}
	})
}
