package diff

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenDiffs(t *testing.T) {
	cases := []struct {
		name, before, after string
		context             int
	}{
		{"added", "", "hello\n世界\n", 3}, {"deleted", "bye\n", "", 3},
		{"modified", "alpha\nbeta\ngamma\n", "alpha\nBETA\ngamma\n", 3},
		{"empty", "", "", 3}, {"no-newline", "old", "new", 3},
		{"newline-only", "same", "same\n", 3},
		{"unicode", "你好\n世界\n", "你好\n世界🙂\n", 3},
		{"multi-hunk", "a\nb\nc\nd\ne\nf\ng\nh\ni\n", "A\nb\nc\nd\ne\nf\ng\nh\nI\n", 1},
		{"zero-context", "a\nb\n", "a\nx\nb\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Defaults()
			opts.ContextLines = tc.context
			r, err := (BoundedLCS{}).Diff(context.Background(), []byte(tc.before), []byte(tc.after), opts)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			if got := Unified(r); got != string(want) {
				t.Fatalf("golden differs\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestDiffLimitsAndStatuses(t *testing.T) {
	cases := []struct {
		name          string
		before, after []byte
		options       Options
		status        Status
	}{
		{"binary", []byte{0}, []byte("x"), Defaults(), Binary},
		{"utf16", nil, []byte{0xff, 0xfe, 'a', 0}, Defaults(), Unsupported},
		{"bytes", nil, []byte("abcd"), Options{MaxBytes: 3, MaxLines: 4, MaxWork: 10}, TooLarge},
		{"lines", nil, []byte("a\nb\n"), Options{MaxBytes: 100, MaxLines: 1, MaxWork: 10}, TooLarge},
		{"work", []byte("a\nb\nc\n"), []byte("x\ny\nz\n"), Options{MaxBytes: 100, MaxLines: 10, MaxWork: 4}, TooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := (BoundedLCS{}).Diff(context.Background(), tc.before, tc.after, tc.options)
			if err != nil || r.Status != tc.status || len(r.Hunks) > 0 {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (BoundedLCS{}).Diff(ctx, nil, nil, Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := (BoundedLCS{}).Diff(context.Background(), nil, nil, Options{}); err == nil {
		t.Fatal("invalid options accepted")
	}
	// Equal prefixes/suffixes make a small edit in a 10k-line file inexpensive.
	before := strings.Repeat("prefix\n", 5000) + "old\n" + strings.Repeat("suffix\n", 4999)
	after := strings.Replace(before, "old\n", "new\n", 1)
	r, err := (BoundedLCS{}).Diff(context.Background(), []byte(before), []byte(after), Defaults())
	if err != nil || r.Status != Text || len(r.Hunks) != 1 {
		t.Fatalf("large simple edit %+v %v", r, err)
	}
}

func FuzzDiffReconstruct(f *testing.F) {
	f.Add("a\nb\n", "a\nx\n")
	f.Add("old", "new")
	f.Add("", "你好\n")
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a) > 512 || len(b) > 512 {
			return
		}
		o := Defaults()
		o.ContextLines = 1000
		r, err := (BoundedLCS{}).Diff(context.Background(), []byte(a), []byte(b), o)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != Text || a == b {
			return
		}
		var old, new bytes.Buffer
		for _, h := range r.Hunks {
			for _, l := range h.Lines {
				suffix := "\n"
				if l.NoNewline {
					suffix = ""
				}
				if l.Kind != Added {
					old.WriteString(l.Text + suffix)
				}
				if l.Kind != Removed {
					new.WriteString(l.Text + suffix)
				}
			}
		}
		if old.String() != a || new.String() != b {
			t.Fatalf("reconstruct failed: before=%q after=%q got=%q/%q", a, b, old.String(), new.String())
		}
	})
}
