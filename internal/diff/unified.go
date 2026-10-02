package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// Unified is a UI-neutral, plain-text representation. Consumers may style the
// structured hunks; no ANSI escape sequences are introduced by this package.
func Unified(r Result) string {
	var b strings.Builder
	if r.Status != Text {
		fmt.Fprintf(&b, "Diff skipped (%s): %s\n", r.Status, r.Reason)
		return b.String()
	}
	before, after := "before", "after"
	if r.Path != "" {
		before = strconv.Quote("a/" + r.Path)
		after = strconv.Quote("b/" + r.Path)
	}
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", before, after)
	for _, h := range r.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			prefix := " "
			if line.Kind == Added {
				prefix = "+"
			} else if line.Kind == Removed {
				prefix = "-"
			}
			fmt.Fprintf(&b, "%s%s\n", prefix, line.Text)
			if line.NoNewline {
				b.WriteString("\\ No newline at end of file\n")
			}
		}
	}
	return b.String()
}
