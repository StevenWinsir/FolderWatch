package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

func TestUnknownBaselineIsExplicitInWatchText(t *testing.T) {
	var out, diagnostics bytes.Buffer
	record := watchRecord{Event: app.Event{Type: "ready", Generation: 1}, State: &changes.View{
		Generation: 1, Version: 1, Changes: []changes.Summary{{Path: "restricted", Kind: changes.Unknown}},
	}}
	if err := renderWatch(&out, &diagnostics, record); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"0 changed file(s), 1 baseline unknown", "? \"restricted\" [Baseline unknown]"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing %q in %q", text, out.String())
		}
	}
}
