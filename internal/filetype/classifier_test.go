package filetype

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestClassificationMatrix(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Kind
	}{
		{"empty", nil, Text}, {"ascii", []byte("hello\n"), Text}, {"unicode", []byte("你好 🙂\n"), Text},
		{"bom-utf8", []byte("\xef\xbb\xbfhello"), Text}, {"crlf", []byte("a\r\nb\t\n"), Text},
		{"nul", []byte{'a', 0, 'b'}, Binary}, {"ansi", []byte("\x1b[31m"), Binary}, {"del", []byte{127}, Binary},
		{"c1", []byte("\u0085"), Binary}, {"invalid", []byte{0xff}, Binary}, {"tail", []byte{0xe4, 0xbd}, Binary},
		{"utf16le", []byte{0xff, 0xfe, 'a', 0}, UnsupportedText}, {"utf16be", []byte{0xfe, 0xff, 0, 'a'}, UnsupportedText},
		{"utf32le", []byte{0xff, 0xfe, 0, 0, 'a', 0, 0, 0}, UnsupportedText}, {"utf32be", []byte{0, 0, 0xfe, 0xff, 0, 0, 0, 'a'}, UnsupportedText},
		{"late-binary", append(bytes.Repeat([]byte("a"), 70000), 0), Binary},
	}
	c := StreamClassifier{MaxBytes: 1 << 20}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.Classify(context.Background(), bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil || got.Kind != tc.want {
				t.Fatalf("got %+v %v want %s", got, err, tc.want)
			}
			var split Probe
			for _, b := range tc.data {
				split.Write([]byte{b})
			}
			if split.Result(int64(len(tc.data)), c.MaxBytes) != got {
				t.Fatal("classification changed with chunk boundaries")
			}
		})
	}
}

type forbiddenReader struct{}

func (forbiddenReader) Read([]byte) (int, error) { panic("oversized content must not be read") }

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

type cancelReader struct{ cancel context.CancelFunc }

func (c cancelReader) Read(p []byte) (int, error) { p[0] = 'a'; c.cancel(); return 1, nil }
func TestBoundedReaderAndErrors(t *testing.T) {
	c := StreamClassifier{MaxBytes: 8}
	got, err := c.Classify(context.Background(), forbiddenReader{}, 1<<40)
	if err != nil || got.Kind != TooLarge {
		t.Fatalf("size %+v %v", got, err)
	}
	for _, pair := range []struct {
		data string
		size int64
	}{{"abc", 2}, {"a", 3}} {
		if _, err := c.Classify(context.Background(), bytes.NewBufferString(pair.data), pair.size); err == nil {
			t.Fatal("size mismatch accepted")
		}
	}
	if _, err := c.Classify(context.Background(), failReader{}, 1); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := c.Classify(ctx, cancelReader{cancel}, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := (StreamClassifier{}).Classify(context.Background(), bytes.NewReader(nil), 0); err == nil {
		t.Fatal("invalid cap")
	}
}

func FuzzClassifier(f *testing.F) {
	for _, data := range [][]byte{nil, []byte("hello"), []byte("你好"), {0xff, 0xfe, 0, 0}, {0xe4, 0xbd}, {0}} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		var a, b Probe
		a.Write(data)
		for i := 0; i < len(data); i += 7 {
			end := i + 7
			if end > len(data) {
				end = len(data)
			}
			b.Write(data[i:end])
		}
		if a.Result(int64(len(data)), 1<<16) != b.Result(int64(len(data)), 1<<16) {
			t.Fatal("chunk-dependent result")
		}
	})
}
