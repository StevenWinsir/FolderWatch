package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/logging"
)

func runWatch(ctx context.Context, prepared app.Prepared, jsonOutput bool, stdout, stderr io.Writer) int {
	log, err := logging.Open(prepared.Config.LogFile, prepared.Config.Root, prepared.Config.Debug)
	if err != nil {
		diagnostic(stderr, err)
		return 1
	}
	defer log.Close()
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
	cursor := watchCursor{}
	logWarningShown := false
	for event := range session.Events() {
		log.Record("debug", "event="+event.Type)
		if event.Type == "warning" {
			log.Record("warning", event.Message)
		}
		if log.Err() != nil && !logWarningShown {
			// A diagnostic only, never mixed into the NDJSON stream.
			logWarningShown = true
			diagnostic(stderr, log.Err())
		}
		record, keep := cursor.project(event, session.ChangeState)
		if !keep {
			continue
		}
		if jsonOutput {
			err = encoder.Encode(record)
		} else {
			err = renderWatch(stdout, stderr, record)
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
