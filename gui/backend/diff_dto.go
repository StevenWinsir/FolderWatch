package backend

import "github.com/StevenWinsir/FolderWatch/internal/diff"

// A conservative JSON upper bound prevents unusually long lines or explicitly
// raised core limits from producing unbounded WebView messages. Escaping costs
// at most six bytes per source byte; object/number overhead is reserved per line.
const maxDiffWireBytes = 16 << 20

func diffDTO(sessionID string, result diff.Result) *DiffResult {
	out := &DiffResult{SessionID: sessionID, Path: result.Path, Kind: result.Kind, Status: string(result.Status), Reason: result.Reason, Generation: decimal(result.Generation), Version: decimal(result.Version), Hunks: []DiffHunk{}}
	cost := 1024 + 6*(len(sessionID)+len(result.Path)+len(result.Reason))
	for _, h := range result.Hunks {
		cost += 256
		for _, line := range h.Lines {
			cost += 128 + 6*len(line.Text)
			if cost > maxDiffWireBytes {
				out.Status, out.Reason = "too-large", "GUI IPC preview exceeds its conservative 16 MiB JSON budget"
				return out
			}
		}
	}
	for _, h := range result.Hunks {
		hunk := DiffHunk{OldStart: h.OldStart, OldLines: h.OldLines, NewStart: h.NewStart, NewLines: h.NewLines, Lines: []DiffLine{}}
		for _, line := range h.Lines {
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: string(line.Kind), OldLine: line.OldLine, NewLine: line.NewLine, Text: line.Text, NoNewline: line.NoNewline})
		}
		out.Hunks = append(out.Hunks, hunk)
	}
	return out
}
