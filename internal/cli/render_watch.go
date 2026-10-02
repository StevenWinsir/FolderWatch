package cli

import (
	"fmt"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

type watchRecord struct {
	app.Event
	State *changes.View `json:"state,omitempty"`
}

func renderWatch(stdout, stderr io.Writer, r watchRecord) error {
	if r.Message != "" {
		if _, err := fmt.Fprintf(stderr, "warning: %q\n", r.Message); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "%s generation=%d sequence=%d\n", r.Type, r.Generation, r.Sequence); err != nil {
		return err
	}
	var upserts []changes.Summary
	if r.State != nil {
		if _, err := fmt.Fprintf(stdout, "%d changed file(s), state version=%d\n", len(r.State.Changes), r.State.Version); err != nil {
			return err
		}
		upserts = r.State.Changes
	} else if r.Batch != nil {
		upserts = r.Batch.Upserts
		for _, path := range r.Batch.Removed {
			if _, err := fmt.Fprintf(stdout, "  = %q (no longer changed)\n", path); err != nil {
				return err
			}
		}
	}
	for _, change := range upserts {
		symbol := map[changes.Kind]string{changes.Added: "A", changes.Modified: "M", changes.Deleted: "D", changes.Renamed: "R"}[change.Kind]
		current := change.After
		if current == nil {
			current = change.Before
		}
		class := ""
		if current != nil {
			class = string(current.Class.Kind)
		}
		if _, err := fmt.Fprintf(stdout, "  %s %q [%s]\n", symbol, change.Path, class); err != nil {
			return err
		}
	}
	return nil
}
