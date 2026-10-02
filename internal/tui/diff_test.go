package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

func plainLines(lines []viewLine) string {
	var result []string
	for _, line := range lines {
		result = append(result, line.text)
	}
	return strings.Join(result, "\n") + "\n"
}
func TestDiffPreviewGolden(t *testing.T) {
	r := diff.Result{Status: diff.Text, Hunks: []diff.Hunk{{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Lines: []diff.Line{{Kind: diff.Context, OldLine: 1, NewLine: 1, Text: "context 世界"}, {Kind: diff.Removed, OldLine: 2, Text: "old"}, {Kind: diff.Added, NewLine: 2, Text: "new", NoNewline: true}}}}}
	lines, err := formatDiff(context.Background(), r, changes.Summary{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "unified.golden")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := plainLines(lines); got != string(want) {
		t.Fatalf("golden mismatch\nGOT:\n%s\nWANT:\n%s", got, want)
	}
	if lines[3].style != "removed" || lines[4].style != "added" {
		t.Fatal("lost color roles")
	}
}
func TestFriendlyStatusesAndEmptyDiff(t *testing.T) {
	for _, test := range []struct {
		status diff.Status
		want   string
	}{{diff.Binary, "Binary file changed"}, {diff.Unsupported, "Unsupported text encoding"}, {diff.TooLarge, "configured size"}, {diff.Unavailable, "No before content was invented"}, {diff.Text, "no differing text lines"}} {
		t.Run(string(test.status), func(t *testing.T) {
			lines, err := formatDiff(context.Background(), diff.Result{Status: test.status}, changes.Summary{})
			if err != nil || !strings.Contains(plainLines(lines), test.want) {
				t.Fatal(plainLines(lines), err)
			}
		})
	}
}
func TestTerminalControlsAndGraphemeClipping(t *testing.T) {
	text := safeText("file\x1b[2J\n\r\x00\u202e世界\tend")
	if strings.ContainsAny(text, "\x1b\n\r\x00\u202e") || !strings.Contains(text, "世界") {
		t.Fatalf("unsafe %q", text)
	}
	if got := cellSlice("A世界B", 1, 3); got != "世 " {
		t.Fatalf("wide crop %q", got)
	}
	if got := cellSlice("Ae\u0301B", 1, 1); got != "e\u0301" {
		t.Fatalf("split grapheme %q", got)
	}
	if got := cellSlice("hello", 0, 0); got != "" {
		t.Fatal(got)
	}
	if len(safeText(strings.Repeat("x", 100000))) > 16450 {
		t.Fatal("unbounded line preview")
	}
}
func TestPreviewBudgetAndCancellation(t *testing.T) {
	r := diff.Result{Status: diff.Text, Hunks: []diff.Hunk{{Lines: make([]diff.Line, 51000)}}}
	for i := range r.Hunks[0].Lines {
		r.Hunks[0].Lines[i] = diff.Line{Kind: diff.Added, Text: "x"}
	}
	lines, err := formatDiff(context.Background(), r, changes.Summary{})
	if err != nil || len(lines) > 50001 || !strings.Contains(lines[len(lines)-1].text, "Preview clipped") {
		t.Fatal(len(lines), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := formatDiff(ctx, r, changes.Summary{}); err == nil {
		t.Fatal("format ignored cancel")
	}
}
func TestNoColorAndSafePathView(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m, f := modelFixture(t, 1)
	if !newPalette(io.Discard).plain {
		t.Fatal("NO_COLOR ignored")
	}
	f.state.Changes[0].Path = "attack\x1b[31m\n\u202e.txt"
	m.acceptView(f.state)
	view := m.View()
	if strings.ContainsAny(view, "\x1b\u202e") {
		t.Fatalf("unsafe view %q", view)
	}
}
