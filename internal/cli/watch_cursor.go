package cli

import (
	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

// A reload can obtain a state newer than events already queued for delivery.
// Remember its watermark so those older deltas cannot roll the CLI backwards.
// This is event projection, not a second filesystem resolver or ChangeStore.
type watchCursor struct{ generation, version uint64 }

func (c *watchCursor) project(event app.Event, view func() changes.View) (watchRecord, bool) {
	event.Paths = nil
	record := watchRecord{Event: event}
	if event.Type == "ready" || event.Type == "reset" || event.Batch != nil && event.Batch.Reload {
		state := view()
		if state.Generation < c.generation || state.Generation == c.generation && state.Version < c.version {
			return watchRecord{}, false
		}
		c.generation, c.version = state.Generation, state.Version
		record.State = &state
		record.Generation = state.Generation
		record.Batch = &changes.Batch{Generation: state.Generation, Version: state.Version, Reload: true}
		return record, true
	}
	if event.Batch != nil {
		batch := event.Batch
		if batch.Generation < c.generation || batch.Generation == c.generation && batch.Version <= c.version {
			return watchRecord{}, false
		}
		c.generation, c.version = batch.Generation, batch.Version
	}
	return record, true
}
