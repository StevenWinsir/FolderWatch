package backend

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestAllChangePagesAndServerSearchAreAccessible(t *testing.T) {
	_, api, client := testFacade(t)
	root := t.TempDir()
	started := api.StartSession(StartOptions{ClientID: client, Root: root})
	if started.Error != nil {
		t.Fatal(started.Error)
	}
	req := SessionRequest{ClientID: client, SessionID: started.Status.SessionID}
	for i := 0; i < 1001; i++ {
		mustWrite(t, filepath.Join(root, fmt.Sprintf("f-%04d.txt", i)), "new file")
	}
	eventually(t, func() bool {
		page := api.GetChanges(ChangesRequest{SessionRequest: req, Limit: 1})
		return page.Error == nil && page.Total == 1001
	})
	first := api.GetChanges(ChangesRequest{SessionRequest: req, Limit: 500})
	if first.Error != nil || first.Total != 1001 || first.Matched != 1001 || first.NextOffset != 500 || len(first.Changes) != 500 {
		t.Fatal(first)
	}
	second := api.GetChanges(ChangesRequest{SessionRequest: req, Offset: 500, Limit: 500, Generation: first.Generation, Version: first.Version})
	if second.Error != nil || second.Changes[0].Path != "f-0500.txt" || second.NextOffset != 1000 {
		t.Fatal(second)
	}
	last := api.GetChanges(ChangesRequest{SessionRequest: req, Offset: 1000, Limit: 500, Generation: first.Generation, Version: first.Version})
	if last.Error != nil || len(last.Changes) != 1 || last.Changes[0].Path != "f-1000.txt" || last.NextOffset != -1 {
		t.Fatal(last)
	}
	all := append(append(first.Changes, second.Changes...), last.Changes...)
	for i, item := range all {
		if item.Path != fmt.Sprintf("f-%04d.txt", i) {
			t.Fatalf("missing/duplicate page member at %d: %s", i, item.Path)
		}
	}
	found := api.GetChanges(ChangesRequest{SessionRequest: req, Limit: 100, Filter: "F-1000"})
	if found.Error != nil || found.Total != 1001 || found.Matched != 1 || len(found.Changes) != 1 || found.Changes[0].Path != "f-1000.txt" {
		t.Fatal(found)
	}
	if reply := api.StopSession(req); reply.Error != nil {
		t.Fatal(reply.Error)
	}
}
