package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/app"
)

func runWatch(ctx context.Context, prepared app.Prepared, jsonOutput bool, stdout, stderr io.Writer) int {
	session, err := app.StartSession(ctx, prepared)
	if err != nil {
		diagnostic(stderr, err)
		if ctx.Err() != nil {
			return 130
		}
		return 1
	}
	defer session.Close()
	encoder := json.NewEncoder(stdout)
	for event := range session.Events() {
		if jsonOutput {
			err = encoder.Encode(event)
		} else {
			_, err = fmt.Fprintf(stdout, "%s generation=%d sequence=%d reconcile=%t baseline_files=%d\n", event.Type, event.Generation, event.Sequence, event.Reconcile, event.BaselineFiles)
			if err == nil {
				for _, p := range event.Paths {
					if _, err = fmt.Fprintf(stdout, "  %q metadata_only=%t\n", p.Path, p.MetadataOnly); err != nil {
						break
					}
				}
			}
			if err == nil && event.Message != "" {
				_, err = fmt.Fprintf(stderr, "warning: %q\n", event.Message)
			}
		}
		if err != nil {
			diagnostic(stderr, err)
			return 1
		}
	}
	if err := session.Close(); err != nil {
		diagnostic(stderr, err)
		return 1
	}
	if ctx.Err() != nil {
		return 130
	}
	return 0
}
