package cli

import (
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
)

func TestReloadWatermarkRejectsQueuedOlderDeltas(t *testing.T) {
	cursor := watchCursor{}
	latest := func() changes.View {
		return changes.View{Generation: 2, Version: 9, Changes: []changes.Summary{{Path: "a", Kind: changes.Modified}}}
	}
	record, keep := cursor.project(app.Event{Type: "reset", Generation: 2, Batch: &changes.Batch{Generation: 2, Version: 5, Reload: true}}, latest)
	if !keep || record.State.Version != 9 || record.Batch.Version != 9 {
		t.Fatal(record)
	}
	for _, batch := range []changes.Batch{{Generation: 1, Version: 99}, {Generation: 2, Version: 7}, {Generation: 2, Version: 9}} {
		if _, keep := cursor.project(app.Event{Type: "changes", Batch: &batch}, latest); keep {
			t.Fatalf("stale delta accepted %+v", batch)
		}
	}
	if _, keep := cursor.project(app.Event{Type: "changes", Batch: &changes.Batch{Generation: 2, Version: 10}}, latest); !keep {
		t.Fatal("new delta rejected")
	}
	if _, keep := cursor.project(app.Event{Type: "warning", Message: "recoverable"}, latest); !keep {
		t.Fatal("warning lost")
	}
}
