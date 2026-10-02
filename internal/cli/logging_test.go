package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/config"
)

type readyWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *readyWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if bytes.Contains(data, []byte(`"type":"ready"`)) {
		w.cancel()
	}
	return n, err
}

func TestLiveLogIsExplicitPrivateAndContentFree(t *testing.T) {
	root, logs := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a"), []byte("PRIVATE-BASELINE-MUST-NOT-BE-LOGGED"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "watch.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &readyWriter{cancel: cancel}
	var stderr bytes.Buffer
	opts := config.LoadOptions{CWD: root, SkipUserConfig: true}
	code := Run(ctx, []string{"--watch", "--json", "--debug", "--log-file", path}, out, &stderr, BuildInfo{}, opts)
	if code != 130 || stderr.Len() != 0 {
		t.Fatal(code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "event=ready") || strings.Contains(string(data), "PRIVATE-BASELINE") {
		t.Fatal(string(data), err)
	}
	if strings.Contains(out.String(), "event=ready") {
		t.Fatal("debug polluted NDJSON")
	}
	for _, destination := range []string{path, filepath.Join(root, "loop.log")} {
		var output bytes.Buffer
		stderr.Reset()
		code = Run(context.Background(), []string{"--watch", "--log-file", destination}, &output, &stderr, BuildInfo{}, opts)
		if code != 1 || output.Len() != 0 || stderr.Len() == 0 {
			t.Fatal(code, output.String(), stderr.String())
		}
	}
}
