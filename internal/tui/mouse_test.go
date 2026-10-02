package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestNoMouseIgnoresEvenPreexistingTerminalReports(t *testing.T) {
	m, _ := modelFixture(t, 5)
	m.noMouse = true
	for _, button := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonWheelDown, tea.MouseButtonWheelUp} {
		_, cmd := m.Update(tea.MouseMsg{X: 4, Y: 5, Button: button, Action: tea.MouseActionPress})
		if cmd != nil || m.selected != 0 || m.expanded {
			t.Fatal("--no-mouse accepted report")
		}
	}
	if m.Init() == nil {
		t.Fatal("no startup disable command")
	}
	press(m, "j")
	if m.selected != 1 {
		t.Fatal("keyboard disabled with mouse")
	}
}
