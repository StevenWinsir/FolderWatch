// Package filetype classifies bytes, never file extensions, before text rendering.
package filetype

import (
	"context"
	"fmt"
	"io"
	"unicode/utf8"
)

type Kind string

const (
	Text            Kind = "text"
	Binary          Kind = "binary"
	UnsupportedText Kind = "unsupported-text"
	TooLarge        Kind = "too-large"
	Unsupported     Kind = "unsupported"
)

// Result is immutable and comparable so it can be stored with an owned snapshot.
type Result struct {
	Kind     Kind   `json:"kind"`
	Encoding string `json:"encoding,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type Classifier interface {
	Classify(context.Context, io.Reader, int64) (Result, error)
}

type StreamClassifier struct{ MaxBytes int64 }

// Probe validates a complete stream using at most three carried UTF-8 bytes.
// The first four bytes are retained only to recognize UTF-16/32 BOMs. It is not
// concurrent-safe; a capture owns its probe, and never shares it with a reader.
type Probe struct {
	prefix    [4]byte
	prefixLen int
	tail      []byte
	binary    bool
}

func (p *Probe) Write(chunk []byte) {
	for _, b := range chunk {
		if p.prefixLen == len(p.prefix) {
			break
		}
		p.prefix[p.prefixLen] = b
		p.prefixLen++
	}
	if p.binary {
		return
	}
	data := chunk
	if len(p.tail) > 0 {
		data = append(p.tail, chunk...)
		p.tail = nil
	}
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			p.tail = append([]byte{}, data...)
			return
		}
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n == 1 || r < 32 && r != '\n' && r != '\r' && r != '\t' || r >= 0x7f && r <= 0x9f {
			p.binary = true
			return
		}
		data = data[n:]
	}
}

// Result prioritizes size, then recognized unsupported encodings, then binary.
// A successful text decision requires the WHOLE stream, not just its prefix.
func (p *Probe) Result(size, maxBytes int64) Result {
	if size > maxBytes {
		return Result{Kind: TooLarge, Reason: "classification byte limit"}
	}
	b := p.prefix
	if p.prefixLen >= 4 && (b == [4]byte{0xff, 0xfe, 0, 0} || b == [4]byte{0, 0, 0xfe, 0xff}) {
		return Result{Kind: UnsupportedText, Encoding: "UTF-32", Reason: "only UTF-8 text is supported"}
	}
	if p.prefixLen >= 2 && (b[0] == 0xff && b[1] == 0xfe || b[0] == 0xfe && b[1] == 0xff) {
		return Result{Kind: UnsupportedText, Encoding: "UTF-16", Reason: "only UTF-8 text is supported"}
	}
	if p.binary || len(p.tail) > 0 {
		return Result{Kind: Binary, Reason: "invalid UTF-8 or control bytes"}
	}
	return Result{Kind: Text, Encoding: "UTF-8"}
}

func (c StreamClassifier) Classify(ctx context.Context, r io.Reader, size int64) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if c.MaxBytes <= 0 || size < 0 || r == nil {
		return Result{}, fmt.Errorf("classification requires a reader, non-negative size and positive byte limit")
	}
	if size > c.MaxBytes {
		return Result{Kind: TooLarge, Reason: "classification byte limit"}, nil
	}
	var p Probe
	var total int64
	buf := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		want := int64(len(buf))
		if size-total < want {
			want = size - total + 1
		}
		n, err := r.Read(buf[:int(want)])
		if n > 0 {
			total += int64(n)
			if total > size {
				return Result{}, fmt.Errorf("file grew while classifying")
			}
			p.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, err
		}
		if n == 0 {
			return Result{}, io.ErrNoProgress
		}
	}
	if total != size {
		return Result{}, io.ErrUnexpectedEOF
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return p.Result(total, c.MaxBytes), nil
}

var _ Classifier = StreamClassifier{}
