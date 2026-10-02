package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/filetype"
)

type rejectWatchWriter struct{}

func (rejectWatchWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestSemanticWatchRenderingAndEscaping(t *testing.T) {
	var out, errs bytes.Buffer
	record := watchRecord{Event: app.Event{Type: "changes", Generation: 1, Batch: &changes.Batch{Upserts: []changes.Summary{{Path: "name\x1b[31m", Kind: changes.Added, After: &changes.FileState{Class: filetype.Result{Kind: filetype.Binary}}}}, Removed: []string{"restored"}}}}
	if err := renderWatch(&out, &errs, record); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(out.String(), '\x1b') || !strings.Contains(out.String(), "[binary]") || !strings.Contains(out.String(), "no longer changed") {
		t.Fatal(out.String())
	}
	if err := renderWatch(rejectWatchWriter{}, &errs, record); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	record.Message = "bad\x1bpath"
	if err := renderWatch(&out, rejectWatchWriter{}, record); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
