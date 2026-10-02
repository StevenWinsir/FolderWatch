package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/config"
)

func TestTUIModeBoundariesAndPipedCompatibility(t *testing.T) {
	for _, args := range [][]string{{"--tui", "--scan"}, {"--tui", "--watch"}, {"--tui", "--json"}} {
		if _, err := Parse(args); err == nil {
			t.Fatal("accepted incompatible modes", args)
		}
	}
	request, err := Parse([]string{"--tui", "--no-mouse"})
	if err != nil || !request.TUI || request.Overlay.NoMouse == nil || !*request.Overlay.NoMouse {
		t.Fatal(request, err)
	}
	var out, stderr bytes.Buffer
	opts := config.LoadOptions{CWD: t.TempDir(), SkipUserConfig: true}
	if code := Run(context.Background(), []string{"--tui"}, &out, &stderr, BuildInfo{}, opts); code != 2 || !strings.Contains(stderr.String(), "requires terminal") {
		t.Fatal(code, stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Run(context.Background(), nil, &out, &stderr, BuildInfo{}, opts); code != 0 || !strings.Contains(out.String(), "initial scan only") {
		t.Fatal(code, out.String(), stderr.String())
	}
}
