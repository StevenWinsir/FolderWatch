package backend

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/diff"
)

func TestSharedGoTypeScriptEventFixture(t *testing.T) {
	fixture, err := os.ReadFile("../frontend/tests/fixtures/ipc-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	want := Event{Protocol: 1, Name: EventChanges, ClientID: "fixture-client", Status: SessionInfo{SessionID: "fixture-session", Root: "/fixture/世界", State: "Monitoring", Sequence: decimal(9007199254740993), Generation: decimal(9007199254740994), Version: decimal(9007199254740995)}, Reload: true}
	generated, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var gotMap, wantMap any
	if err := json.Unmarshal(fixture, &gotMap); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(generated, &wantMap); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("wire contract drift:\nfixture %s\nGo DTO %s", fixture, generated)
	}
}

func TestDiffWireBudgetFallsBackWithoutContent(t *testing.T) {
	result := diff.Result{Path: "large.txt", Kind: "modified", Status: diff.Text, Generation: 1, Version: 2, Hunks: []diff.Hunk{{Lines: []diff.Line{{Kind: diff.Added, Text: strings.Repeat("PRIVATE", maxDiffWireBytes/42+1)}}}}}
	out := diffDTO("session", result)
	if out.Status != "too-large" || len(out.Hunks) != 0 || out.Generation != "1" || out.Version != "2" {
		t.Fatalf("unbounded IPC diff: %+v", out)
	}
	b, err := json.Marshal(out)
	if err != nil || len(b) > 1024 || strings.Contains(string(b), "PRIVATE") {
		t.Fatalf("fallback leaked contents: %v %s", err, b)
	}
}
