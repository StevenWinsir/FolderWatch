package backend

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
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
	// This is a 1001-record functional coverage assertion, not the small-
	// fixture helper's five-second latency budget. Go 1.23 race instrumentation
	// on a shared CI runner can exceed that budget while still converging.
	// Retain live watcher delivery (no forced rescan), all row assertions, and
	// a bounded deadline with actionable diagnostics instead of a silent retry.
	waitStarted := time.Now()
	deadline := waitStarted.Add(30 * time.Second)
	for {
		if heartbeat := api.Heartbeat(client); heartbeat.Error != nil {
			t.Fatalf("frontend lease lost while waiting for coverage: %+v", heartbeat.Error)
		}
		page := api.GetChanges(ChangesRequest{SessionRequest: req, Limit: 1})
		if page.Error != nil && page.Error.Code != "STALE_VERSION" && page.Error.Code != "BUSY" {
			t.Fatalf("coverage query failed: %+v", page.Error)
		}
		if page.Error == nil && page.Total == 1001 {
			t.Logf("1001 live changes converged in %s", time.Since(waitStarted))
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("incomplete live coverage after %s: total=%d error=%+v status=%+v", time.Since(waitStarted), page.Total, page.Error, api.GetSessionStatus(client))
		}
		time.Sleep(25 * time.Millisecond)
	}
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
