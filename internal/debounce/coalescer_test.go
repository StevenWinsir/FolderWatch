package debounce

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

type filter struct{}

func (filter) Match(path string, isDir bool) (bool, error) { return path == "ignored", nil }
func start(t *testing.T, cap int) (chan watcher.RawEvent, <-chan Batch, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	in := make(chan watcher.RawEvent, 128)
	out, err := Start(ctx, t.TempDir(), filter{}, in, Options{Delay: 20 * time.Millisecond, MaxPending: cap})
	if err != nil {
		t.Fatal(err)
	}
	return in, out, cancel
}
func next(t *testing.T, out <-chan Batch) Batch {
	t.Helper()
	select {
	case b, ok := <-out:
		if !ok {
			t.Fatal("closed early")
		}
		return b
	case <-time.After(3 * time.Second):
		t.Fatal("batch timeout")
		return Batch{}
	}
}
func quiet(t *testing.T, out <-chan Batch) {
	t.Helper()
	select {
	case b := <-out:
		t.Fatalf("duplicate batch %+v", b)
	case <-time.After(60 * time.Millisecond):
	}
}

func TestCoalesceSortedAndMetadataOnly(t *testing.T) {
	in, out, _ := start(t, 16)
	for i := 0; i < 30; i++ {
		in <- watcher.RawEvent{Path: "b", Op: watcher.Write}
		in <- watcher.RawEvent{Path: "b", Op: watcher.Chmod}
	}
	in <- watcher.RawEvent{Path: "a", Op: watcher.Chmod}
	in <- watcher.RawEvent{Path: "ignored", Op: watcher.Write}
	b := next(t, out)
	if b.Reconcile || len(b.Paths) != 2 || b.Paths[0].Path != "a" || !b.Paths[0].MetadataOnly || b.Paths[1].MetadataOnly {
		t.Fatalf("batch %+v", b)
	}
	quiet(t, out)
}
func TestPendingOverflowAndSlowConsumer(t *testing.T) {
	in, out, _ := start(t, 2)
	for _, p := range []string{"a", "b", "c"} {
		in <- watcher.RawEvent{Path: p, Op: watcher.Create}
	}
	if b := next(t, out); !b.Reconcile {
		t.Fatalf("overflow %+v", b)
	}
	in <- watcher.RawEvent{Path: "first", Op: watcher.Write}
	deadline := time.After(3 * time.Second)
	for len(out) == 0 {
		select {
		case <-deadline:
			t.Fatal("no output")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	in <- watcher.RawEvent{Path: "second", Op: watcher.Write}
	time.Sleep(80 * time.Millisecond)
	_ = next(t, out)
	if b := next(t, out); !b.Reconcile {
		t.Fatalf("backpressure lost invalidation %+v", b)
	}
}
func TestContinuousWritesHaveMaximumWait(t *testing.T) {
	in, out, cancel := start(t, 16)
	done := make(chan struct{})
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); <-done })
	go func() {
		defer close(done)
		timer := time.NewTicker(time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				select {
				case in <- watcher.RawEvent{Path: "busy", Op: watcher.Write}:
				default:
				}
			case <-stop:
				return
			}
		}
	}()
	began := time.Now()
	b := next(t, out)
	if len(b.Paths) != 1 || b.Paths[0].Path != "busy" {
		t.Fatalf("busy %+v", b)
	}
	if time.Since(began) > 500*time.Millisecond {
		t.Fatal("continuous saves starved the maximum wait")
	}
	cancel()
}

func TestCancellationAndValidation(t *testing.T) {
	for i := 0; i < 20; i++ {
		in, out, cancel := start(t, 4)
		in <- watcher.RawEvent{Path: "pending", Op: watcher.Write}
		cancel()
		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-time.After(time.Second):
			t.Fatal("coalescer leaked")
		}
	}
	root := t.TempDir()
	for _, opts := range []Options{{}, {Delay: time.Second, MaxPending: 0}, {Delay: time.Duration(1<<63 - 1), MaxPending: 1}} {
		if _, err := Start(context.Background(), root, filter{}, nil, opts); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan watcher.RawEvent, 1)
	out, err := Start(ctx, root, filter{}, in, Options{Delay: time.Millisecond, MaxPending: 2})
	if err != nil {
		t.Fatal(err)
	}
	in <- watcher.RawEvent{Path: filepath.Join(root, "gone"), Op: watcher.Remove}
	if b := next(t, out); len(b.Paths) != 1 || b.Paths[0].Path != "gone" {
		t.Fatalf("deleted key %+v", b)
	}
}
