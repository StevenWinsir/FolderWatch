package tui

import (
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/app"
)

func TestDelayedStatusSnapshotsDoNotReverseControls(t *testing.T) {
	m, f := modelFixture(t, 0)
	beforePause := eventMsg{state: f.state, status: app.Monitoring}
	execute(t, m, press(m, "p"))
	if m.status != app.Paused {
		t.Fatal(m.status)
	}
	m.Update(beforePause)
	if m.status != app.Paused {
		t.Fatalf("delayed event reversed pause: %s", m.status)
	}
	beforeResume := eventMsg{state: f.state, status: app.Paused}
	execute(t, m, press(m, "p"))
	if got := f.actions[len(f.actions)-1]; got != "resume" {
		t.Fatalf("p selected %s instead of resume", got)
	}
	m.Update(beforeResume)
	if m.status != app.Monitoring {
		t.Fatalf("delayed event reversed resume: %s", m.status)
	}
	// A captured control response must not hide a later terminal core state.
	f.status = app.Error
	m.Update(controlMsg{action: "resume", state: f.state, status: app.Monitoring})
	if m.status != app.Error {
		t.Fatalf("delayed control response hid terminal status: %s", m.status)
	}
}
