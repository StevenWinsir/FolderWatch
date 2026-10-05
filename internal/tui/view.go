package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/charmbracelet/lipgloss"
	"github.com/rivo/uniseg"
)

type palette struct {
	plain                                   bool
	title, added, removed, accent, selected lipgloss.Style
}

func newPalette(output io.Writer) palette {
	r := lipgloss.NewRenderer(output)
	_, noColor := os.LookupEnv("NO_COLOR")
	return palette{plain: noColor || os.Getenv("TERM") == "dumb", title: r.NewStyle().Bold(true), added: r.NewStyle().Foreground(lipgloss.Color("2")), removed: r.NewStyle().Foreground(lipgloss.Color("1")), accent: r.NewStyle().Foreground(lipgloss.Color("6")), selected: r.NewStyle().Reverse(true)}
}
func (p palette) paint(text, kind string) string {
	if p.plain {
		return text
	}
	switch kind {
	case "title":
		return p.title.Render(text)
	case "added":
		return p.added.Render(text)
	case "removed":
		return p.removed.Render(text)
	case "accent":
		return p.accent.Render(text)
	case "selected":
		return p.selected.Render(text)
	}
	return text
}

// safeText neutralizes terminal escapes, line breaks, bidi/format controls and
// malformed UTF-8. Tabs use fixed spaces; long logical lines have a preview cap.
func safeText(text string) string {
	var out strings.Builder
	for _, r := range text {
		if out.Len() >= 16384 {
			out.WriteString(" … [line preview clipped]")
			break
		}
		if r == '\t' {
			out.WriteString("    ")
		} else if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			out.WriteString(strconv.QuoteRuneToASCII(r))
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// cellSlice crops plain text by terminal cells without splitting a grapheme.
// Styling is applied afterwards, so content can never inject terminal commands.
func cellSlice(text string, left, width int) string {
	if width <= 0 {
		return ""
	}
	g := uniseg.NewGraphemes(text)
	position := 0
	var out strings.Builder
	for g.Next() {
		w := g.Width()
		next := position + w
		if next > left && position < left+width {
			if position < left || next > left+width {
				out.WriteString(strings.Repeat(" ", max(0, min(next, left+width)-max(position, left))))
			} else {
				out.WriteString(g.Str())
			}
		}
		position = next
		if position >= left+width {
			break
		}
	}
	return out.String()
}

var helpLines = []string{
	"KEYBOARD — mouse is optional",
	"↑/↓ or j/k     Select a changed file",
	"Enter / Space  Expand or collapse selected file",
	"PgUp/PgDn      Scroll diff (or page the collapsed list)",
	"Ctrl+u/Ctrl+d  Scroll diff one page",
	"Home/End g/G   First/last diff page (or file)",
	"←/→ or h/l     Horizontal diff scroll",
	"/             Filter paths (case-insensitive); Enter applies",
	"Esc           Cancel filter edit / clear applied filter",
	"p             Pause / reconcile and resume",
	"r             Reset baseline; y/Enter confirms, other keys cancel",
	"e             Bounded diagnostics, including recoverable errors",
	"?             Help; ↑/↓ scroll this page; Esc closes",
	"q / Ctrl+C    Quit (0) / cancel (130), restoring the terminal",
	"",
	"A Added · M Modified · D Deleted · R Renamed",
	"? Baseline unknown — unreadable at startup; complete Reset establishes a baseline.",
	"[+] collapsed / [-] expanded are not change-kind markers.",
	"Pause freezes the list, not the watcher. Resume compares to the same baseline.",
	"Only one selected diff is retained. File/version changes cancel old requests.",
	"Binary/unsupported/large files remain visible; unavailable before is never invented.",
	"No file content is logged. --debug adds metadata to the 200-entry ring.",
	"NO_COLOR disables styling. --no-mouse disables mouse reporting.",
}

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	var rows []string
	add := func(text, style string) { rows = append(rows, m.palette.paint(cellSlice(text, 0, m.width), style)) }
	if m.width < 40 || m.height < 12 {
		add("FolderWatch — "+string(m.status), "title")
		add("Resize to at least 40 columns × 12 rows.", "")
		add("q quit · Ctrl+C cancel", "")
		return strings.Join(rows[:min(len(rows), m.height)], "\n")
	}
	add("FolderWatch | "+string(m.status)+" | "+safeText(m.root), "title")
	unknown := 0
	for _, item := range m.state.Changes {
		if item.Kind == changes.Unknown {
			unknown++
		}
	}
	if unknown > 0 {
		add(fmt.Sprintf("%d files changed · %d baseline unknown · %d shown · baseline %d · /%s", len(m.state.Changes)-unknown, unknown, len(m.visible), m.state.Generation, safeText(m.filter)), "accent")
	} else {
		add(fmt.Sprintf("%d files changed · %d shown · baseline %d · /%s", len(m.state.Changes), len(m.visible), m.state.Generation, safeText(m.filter)), "accent")
	}
	if m.help || m.diagnostics {
		lines, title := helpLines, "Help — ↑/↓ scroll · Esc close"
		if m.diagnostics {
			lines, title = m.diagnosticLines, "Diagnostics — ↑/↓ scroll · Esc close"
		}
		add(title, "title")
		for i := 0; i < m.height-4; i++ {
			index := m.overlayTop + i
			text := ""
			if index < len(lines) {
				text = lines[index]
			}
			add(text, "")
		}
		add("q quit · Ctrl+C cancel", "accent")
		return strings.Join(rows, "\n")
	}
	add("Changed files  [Enter expand/collapse]", "title")
	for i := 0; i < m.listHeight(); i++ {
		index := m.listTop + i
		text, style := "", ""
		if index < len(m.visible) {
			item := m.visible[index]
			marker := "[+]"
			cursor := " "
			if index == m.selected {
				cursor = ">"
				style = "selected"
				if m.expanded {
					marker = "[-]"
				}
			}
			kind := map[changes.Kind]string{changes.Added: "A", changes.Modified: "M", changes.Deleted: "D", changes.Renamed: "R", changes.Unknown: "?"}[item.Kind]
			text = fmt.Sprintf("%s %s %s %s", cursor, marker, kind, safeText(item.Path))
		} else if i == 0 {
			text = "No changes. Edit a file in the monitored folder."
			if m.filter != "" {
				text = "No changed paths match this filter. Esc clears it."
			}
		}
		add(text, style)
	}
	header := "Diff — Enter to expand selected file"
	if item, ok := m.current(); ok && m.expanded {
		header = fmt.Sprintf("Diff %s · lines %d/%d · PgUp/PgDn · ←/→", safeText(item.Path), min(m.diffTop+1, len(m.diffLines)), len(m.diffLines))
	}
	add(header, "accent")
	for i := 0; i < m.diffHeight(); i++ {
		text, style := "", ""
		index := m.diffTop + i
		if m.expanded && index < len(m.diffLines) {
			line := m.diffLines[index]
			text = cellSlice(line.text, m.diffLeft, m.width)
			style = line.style
		} else if i == 0 && m.expanded && m.diffRunning {
			text = "Loading diff… navigation and q remain available."
		}
		add(text, style)
	}
	notice := m.notice
	if m.filtering {
		notice = "Filter: /" + m.filter + "  (Enter applies · Esc cancels)"
	}
	add(safeText(notice), "")
	add("↑↓ j/k select · Enter diff · / filter · p pause · r reset · ? help · e details · q quit", "accent")
	return strings.Join(rows, "\n")
}
