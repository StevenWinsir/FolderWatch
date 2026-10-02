package tui

import (
	"context"
	"fmt"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

type viewLine struct{ text, style string }

// Formatting occurs in the single cancellable diff command, not View/Update.
// In addition to core compute limits the terminal preview is bounded to 16MiB,
// 50,000 rows and 16KiB per logical line. Only visible rows are styled per frame.
func formatDiff(ctx context.Context, result diff.Result, item changes.Summary) ([]viewLine, error) {
	var lines []viewLine
	bytes := 0
	appendLine := func(text, style string) { lines = append(lines, viewLine{text, style}); bytes += len(text) }
	size := func(state *changes.FileState) string {
		if state == nil {
			return "absent"
		}
		return fmt.Sprintf("%d bytes", state.Meta.Size)
	}
	appendLine("Size: "+size(item.Before)+" → "+size(item.After), "")
	if result.Status != diff.Text {
		message := map[diff.Status]string{diff.Binary: "Binary file changed; no text diff.", diff.Unsupported: "Unsupported text encoding or file type; no text diff.", diff.TooLarge: "File changed; diff skipped because a configured size, line or computation limit was exceeded.", diff.Unavailable: "File changed; baseline/current content is unavailable. No before content was invented."}[result.Status]
		appendLine(message, "accent")
		appendLine(safeText(result.Reason), "")
		return lines, ctx.Err()
	}
	if len(result.Hunks) == 0 {
		appendLine("Text file changed; no differing text lines (for example an empty added/deleted file).", "")
	}
	for _, hunk := range result.Hunks {
		appendLine(fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines), "accent")
		for _, line := range hunk.Lines {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if bytes >= 16<<20 || len(lines) >= 50000 {
				appendLine("Preview clipped at the terminal display budget; refine diff limits or inspect the file externally.", "accent")
				return lines, nil
			}
			prefix, style := " ", ""
			if line.Kind == diff.Added {
				prefix, style = "+", "added"
			}
			if line.Kind == diff.Removed {
				prefix, style = "-", "removed"
			}
			old, new := "", ""
			if line.OldLine > 0 {
				old = fmt.Sprint(line.OldLine)
			}
			if line.NewLine > 0 {
				new = fmt.Sprint(line.NewLine)
			}
			appendLine(fmt.Sprintf("%6s %6s %s%s", old, new, prefix, safeText(line.Text)), style)
			if line.NoNewline {
				appendLine("              \\ No newline at end of file", "accent")
			}
		}
	}
	return lines, ctx.Err()
}
