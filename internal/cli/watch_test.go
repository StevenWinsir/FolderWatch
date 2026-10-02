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

func TestWatchCLIStreamsAndCancels(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	code := Run(ctx, []string{"--watch", "--json", root}, &out, &errOut, BuildInfo{}, config.LoadOptions{SkipUserConfig: true})
	if code != 130 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
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
