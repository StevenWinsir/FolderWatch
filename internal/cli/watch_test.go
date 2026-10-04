package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/config"
)

type failWatchWriter struct{}

func (failWatchWriter) Write([]byte) (int, error) { return 0, errors.New("intentional output failure") }

type cancelAfterReadyWriter struct {
	out    *bytes.Buffer
	cancel context.CancelFunc
	ready  bool
}

func (w *cancelAfterReadyWriter) Write(data []byte) (int, error) {
	n, err := w.out.Write(data)
	if err != nil || w.ready {
		return n, err
	}
	if end := bytes.IndexByte(w.out.Bytes(), '\n'); end >= 0 {
		var event struct{ Type string }
		if json.Unmarshal(w.out.Bytes()[:end], &event) == nil && event.Type == "ready" {
			// Cancel only after the complete ready record has actually been
			// written. There is no assumption about startup or disk speed.
			w.ready = true
			w.cancel()
		}
	}
	return n, err
}

func TestWatchCLIStreamsAndCancels(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	writer := &cancelAfterReadyWriter{out: &out, cancel: cancel}
	code := Run(ctx, []string{"--watch", "--json", root}, writer, &errOut, BuildInfo{}, config.LoadOptions{SkipUserConfig: true})
	if code != 130 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if !writer.ready {
		t.Fatalf("watchdog expired before ready; stdout=%s stderr=%s", out.String(), errOut.String())
	}
	var event struct {
		Type       string
		Generation int
		Reconcile  bool
	}
	if err := json.NewDecoder(&out).Decode(&event); err != nil || event.Type != "ready" || event.Generation != 1 || !event.Reconcile {
		t.Fatalf("event=%+v %v", event, err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run(context.Background(), []string{"--watch", root}, failWatchWriter{}, &errOut, BuildInfo{}, config.LoadOptions{SkipUserConfig: true}); code != 1 {
		t.Fatalf("output failure exit=%d", code)
	}
	for _, args := range [][]string{{"--watch", "--scan", root}, {"--watch", "--max-pending-events=0", root}, {"--watch", "--snapshot-cache-bytes=bad", root}, {"--watch", "--debounce=2562047h", root}} {
		if code := Run(context.Background(), args, &out, &errOut, BuildInfo{}, config.LoadOptions{SkipUserConfig: true}); code != 2 {
			t.Fatalf("%v exit=%d", args, code)
		}
	}
}

func TestWatchCLICancelledStartupDoesNotInventReady(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	code := Run(ctx, []string{"--watch", "--json", root}, &out, &errOut, BuildInfo{}, config.LoadOptions{SkipUserConfig: true})
	if code != 130 || out.Len() != 0 {
		t.Fatalf("cancelled startup: exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}
