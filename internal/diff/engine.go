package diff

import (
	"bytes"
	"context"
	"strings"

	"github.com/StevenWinsir/FolderWatch/internal/filetype"
)

// BoundedLCS trims equal edges, then uses an explicitly capped LCS matrix.
// No third-party uncancellable computation or detached timeout goroutine occurs.
// Difficult inputs return TooLarge instead of allocating an unbounded matrix.
type BoundedLCS struct{}

func (BoundedLCS) Diff(ctx context.Context, before, after []byte, o Options) (Result, error) {
	result := Result{Status: Text, Hunks: []Hunk{}}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := o.Validate(); err != nil {
		return Result{}, err
	}
	skip := func(reason string) (Result, error) {
		return Result{Status: TooLarge, Reason: reason, Hunks: []Hunk{}}, nil
	}
	if int64(len(before)) > o.MaxBytes || int64(len(after)) > o.MaxBytes {
		return skip("diff byte limit")
	}
	classifier := filetype.StreamClassifier{MaxBytes: o.MaxBytes}
	for _, data := range [][]byte{before, after} {
		class, err := classifier.Classify(ctx, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return Result{}, err
		}
		if class.Kind != filetype.Text {
			status := Binary
			if class.Kind == filetype.UnsupportedText {
				status = Unsupported
			}
			return Result{Status: status, Reason: class.Reason, Hunks: []Hunk{}}, nil
		}
	}
	count := func(b []byte) int {
		n := bytes.Count(b, []byte{'\n'})
		if len(b) > 0 && b[len(b)-1] != '\n' {
			n++
		}
		return n
	}
	if count(before) > o.MaxLines || count(after) > o.MaxLines {
		return skip("diff line limit")
	}
	if bytes.Equal(before, after) {
		return result, nil
	}
	split := func(b []byte) []string {
		if len(b) == 0 {
			return nil
		}
		lines := strings.SplitAfter(string(b), "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		return lines
	}
	a, b := split(before), split(after)
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
		if prefix%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
		if suffix%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
		}
	}
	x, y := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	// Intern lines once: the quadratic loop compares integers, not repeatedly
	// hashing/comparing potentially very long line contents.
	ids := make(map[string]int)
	intern := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, line := range lines {
			id, ok := ids[line]
			if !ok {
				id = len(ids) + 1
				ids[line] = id
			}
			out[i] = id
		}
		return out
	}
	xid, yid := intern(x), intern(y)
	var matrix []uint32
	width := len(y) + 1
	if len(x) > 0 && len(y) > 0 {
		cells := int64(len(x)+1) * int64(width)
		if cells > o.MaxWork {
			return skip("diff computation limit")
		}
		matrix = make([]uint32, int(cells))
		for i := len(x) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			for j := len(y) - 1; j >= 0; j-- {
				if j%256 == 0 {
					if err := ctx.Err(); err != nil {
						return Result{}, err
					}
				}
				if xid[i] == yid[j] {
					matrix[i*width+j] = matrix[(i+1)*width+j+1] + 1
				} else {
					left, down := matrix[i*width+j+1], matrix[(i+1)*width+j]
					if down >= left {
						matrix[i*width+j] = down
					} else {
						matrix[i*width+j] = left
					}
				}
			}
		}
	}
	lines := make([]Line, 0, len(a)+len(b))
	oldLine, newLine := 1, 1
	emit := func(kind LineKind, text string) {
		line := Line{Kind: kind, Text: strings.TrimSuffix(text, "\n"), NoNewline: !strings.HasSuffix(text, "\n")}
		if kind != Added {
			line.OldLine = oldLine
			oldLine++
		}
		if kind != Removed {
			line.NewLine = newLine
			newLine++
		}
		lines = append(lines, line)
	}
	for i := 0; i < prefix; i++ {
		emit(Context, a[i])
	}
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if i < len(x) && j < len(y) && xid[i] == yid[j] {
			emit(Context, x[i])
			i++
			j++
		} else if j == len(y) || (i < len(x) && (len(matrix) == 0 || matrix[(i+1)*width+j] >= matrix[i*width+j+1])) {
			emit(Removed, x[i])
			i++
		} else {
			emit(Added, y[j])
			j++
		}
	}
	for i := len(a) - suffix; i < len(a); i++ {
		emit(Context, a[i])
	}
	result.Hunks = group(lines, o.ContextLines)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func group(lines []Line, contextLines int) []Hunk {
	type span struct{ start, end int }
	var spans []span
	for i, line := range lines {
		if line.Kind == Context {
			continue
		}
		start, end := i-contextLines, i+contextLines+1
		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		if len(spans) > 0 && start <= spans[len(spans)-1].end {
			if end > spans[len(spans)-1].end {
				spans[len(spans)-1].end = end
			}
		} else {
			spans = append(spans, span{start, end})
		}
	}
	out := make([]Hunk, 0, len(spans))
	oldBefore, newBefore, pos := 0, 0, 0
	for _, s := range spans {
		for pos < s.start {
			if lines[pos].Kind != Added {
				oldBefore++
			}
			if lines[pos].Kind != Removed {
				newBefore++
			}
			pos++
		}
		h := Hunk{OldStart: oldBefore + 1, NewStart: newBefore + 1, Lines: append([]Line{}, lines[s.start:s.end]...)}
		for _, line := range h.Lines {
			if line.Kind != Added {
				h.OldLines++
			}
			if line.Kind != Removed {
				h.NewLines++
			}
		}
		if h.OldLines == 0 {
			h.OldStart = oldBefore
		}
		if h.NewLines == 0 {
			h.NewStart = newBefore
		}
		out = append(out, h)
	}
	return out
}

var _ Engine = BoundedLCS{}
