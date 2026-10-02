package cli

import (
	"context"
	"encoding/json"
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
	cursor := watchCursor{}
	for event := range session.Events() {
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
