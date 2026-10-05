package tui

import (
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

func TestUnknownBaselineIsExplicitInTerminalListAndHelp(t *testing.T) {
	m, backend := modelFixture(t, 1)
	backend.state.Changes[0].Kind = changes.Unknown
	m.acceptView(backend.state)
	if view := m.View(); !strings.Contains(view, "[+] ? file-000.txt") || !strings.Contains(view, "1 baseline unknown") {
		t.Fatalf("unknown baseline must have a visible marker and count: %s", view)
	}
	if !strings.Contains(strings.Join(helpLines, "\n"), "? Baseline unknown") {
		t.Fatal("help must explain the unknown-baseline marker")
	}
}
