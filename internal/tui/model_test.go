package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/logging"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fakeBackend struct {
	state      changes.View
	status     app.Status
	events     chan app.Event
	diffFn     func(context.Context, string) (diff.Result, error)
	actions    []string
	controlErr error
}

func (f *fakeBackend) Events() <-chan app.Event  { return f.events }
func (f *fakeBackend) ChangeState() changes.View { return f.state }
func (f *fakeBackend) Status() app.Status        { return f.status }
func (f *fakeBackend) Err() error                { return nil }
func (f *fakeBackend) Close() error              { return nil }
func (f *fakeBackend) GetDiff(ctx context.Context, path string) (diff.Result, error) {
	return f.diffFn(ctx, path)
}
func (f *fakeBackend) Pause(context.Context) error {
	f.actions = append(f.actions, "pause")
	if f.controlErr == nil {
		f.status = app.Paused
	}
	return f.controlErr
}
func (f *fakeBackend) Resume(context.Context) error {
	f.actions = append(f.actions, "resume")
	if f.controlErr == nil {
		f.status = app.Monitoring
	}
	return f.controlErr
}
func (f *fakeBackend) ResetBaseline(context.Context) error {
	f.actions = append(f.actions, "reset")
	if f.controlErr == nil {
		f.state = changes.View{Generation: f.state.Generation + 1}
	}
	return f.controlErr
}

func modelFixture(t *testing.T, count int) (*Model, *fakeBackend) {
	t.Helper()
	log, err := logging.Open("", t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeBackend{status: app.Monitoring, events: make(chan app.Event), state: changes.View{Generation: 1, Version: 1}}
	for i := 0; i < count; i++ {
		f.state.Changes = append(f.state.Changes, changes.Summary{Path: fmt.Sprintf("file-%03d.txt", i), Kind: changes.Modified, Version: 1})
	}
	f.diffFn = func(ctx context.Context, path string) (diff.Result, error) {
		return diff.Result{Path: path, Generation: 1, Version: 1, Status: diff.Text, Hunks: []diff.Hunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{{Kind: diff.Removed, OldLine: 1, Text: "before " + path}, {Kind: diff.Added, NewLine: 1, Text: "after " + path}}}}}, nil
	}
	m := newModel(context.Background(), "/project", log, io.Discard)
	m.palette.plain = true
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(startedMsg{service: f})
	return m, f
}
func press(m *Model, name string) tea.Cmd {
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	switch name {
	case "enter":
		key.Type = tea.KeyEnter
	case "space":
		key.Type = tea.KeySpace
	case "up":
		key.Type = tea.KeyUp
	case "down":
		key.Type = tea.KeyDown
	case "pgdown":
		key.Type = tea.KeyPgDown
	case "pgup":
		key.Type = tea.KeyPgUp
	case "home":
		key.Type = tea.KeyHome
	case "end":
		key.Type = tea.KeyEnd
	case "right":
		key.Type = tea.KeyRight
	case "left":
		key.Type = tea.KeyLeft
	case "esc":
		key.Type = tea.KeyEsc
	case "backspace":
		key.Type = tea.KeyBackspace
	case "ctrl+c":
		key.Type = tea.KeyCtrlC
	}
	_, cmd := m.Update(key)
	return cmd
}
func execute(t *testing.T, m *Model, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	_, next := m.Update(cmd())
	return next
}

func TestListCountsResizeAndCellBounds(t *testing.T) {
	for _, count := range []int{0, 1, 120} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			m, _ := modelFixture(t, count)
			for _, width := range []int{0, 1, 39, 40, 80, 160} {
				for _, height := range []int{0, 1, 11, 12, 24, 60} {
					m.Update(tea.WindowSizeMsg{Width: width, Height: height})
					view := m.View()
					if width == 0 || height == 0 {
						if view != "" {
							t.Fatal("zero size")
						}
						continue
					}
					lines := strings.Split(view, "\n")
					if len(lines) > height {
						t.Fatalf("overflow %dx%d: %d", width, height, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) > width {
							t.Fatalf("line too wide %d: %q", width, line)
						}
					}
				}
			}
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			if !strings.Contains(m.View(), fmt.Sprintf("%d files changed", count)) {
				t.Fatal(m.View())
			}
		})
	}
}

func TestKeyboardSelectionFilterAndHelp(t *testing.T) {
	m, _ := modelFixture(t, 120)
	press(m, "j")
	press(m, "down")
	press(m, "k")
	if m.selected != 1 {
		t.Fatal(m.selected)
	}
	press(m, "end")
	if m.selected != 119 || m.listTop == 0 {
		t.Fatal(m.selected, m.listTop)
	}
	press(m, "home")
	if m.selected != 0 {
		t.Fatal(m.selected)
	}
	press(m, "pgdown")
	if m.selected != m.listHeight() {
		t.Fatal(m.selected)
	}
	press(m, "/")
	press(m, "100")
	if len(m.visible) != 1 || m.visible[0].Path != "file-100.txt" {
		t.Fatal(m.visible)
	}
	press(m, "enter")
	if m.filtering {
		t.Fatal("filter submit")
	}
	press(m, "esc")
	if len(m.visible) != 120 {
		t.Fatal("filter clear")
	}
	press(m, "/")
	if cmd := press(m, "q"); cmd != nil {
		t.Fatal("q should type in filter")
	}
	press(m, "esc")
	if m.filter != "" || len(m.visible) != 120 {
		t.Fatal("filter cancel")
	}
	press(m, "?")
	if !m.help || !strings.Contains(m.View(), "KEYBOARD") {
		t.Fatal(m.View())
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	press(m, "pgdown")
	if m.overlayTop == 0 {
		t.Fatal("help does not scroll")
	}
	press(m, "esc")
	if m.help {
		t.Fatal("help close")
	}
	if _, ok := press(m, "q")().(tea.QuitMsg); !ok {
		t.Fatal("q")
	}
	m, _ = modelFixture(t, 0)
	press(m, "ctrl+c")
	if !m.interrupted {
		t.Fatal("cancel exit code")
	}
}

func TestDiffThousandLinesScrollingAndCollapse(t *testing.T) {
	m, f := modelFixture(t, 1)
	f.diffFn = func(context.Context, string) (diff.Result, error) {
		r := diff.Result{Path: "file-000.txt", Generation: 1, Version: 1, Status: diff.Text}
		h := diff.Hunk{NewStart: 1, NewLines: 1500}
		for i := 1; i <= 1500; i++ {
			h.Lines = append(h.Lines, diff.Line{Kind: diff.Added, NewLine: i, Text: fmt.Sprintf("row-%04d", i)})
		}
		r.Hunks = []diff.Hunk{h}
		return r, nil
	}
	cmd := press(m, "enter")
	if !strings.Contains(m.View(), "Loading diff") {
		t.Fatal(m.View())
	}
	execute(t, m, cmd)
	if len(m.diffLines) != 1502 || !strings.Contains(m.View(), "+row-0001") {
		t.Fatal(len(m.diffLines), m.View())
	}
	press(m, "pgdown")
	if m.diffTop == 0 {
		t.Fatal("scroll")
	}
	press(m, "end")
	if !strings.Contains(m.View(), "row-1500") {
		t.Fatal(m.View())
	}
	press(m, "home")
	press(m, "right")
	if m.diffTop != 0 || m.diffLeft != 8 {
		t.Fatal("home/horizontal")
	}
	press(m, "left")
	press(m, "space")
	if m.expanded || m.diffLines != nil {
		t.Fatal("collapse retained diff")
	}
}

func TestSingleDiffLaneAndStaleSelectionRejection(t *testing.T) {
	m, _ := modelFixture(t, 3)
	first := press(m, "enter")
	if cmd := press(m, "j"); cmd != nil {
		t.Fatal("unbounded second diff worker")
	}
	if cmd := press(m, "j"); cmd != nil {
		t.Fatal("unbounded third diff worker")
	}
	next := execute(t, m, first)
	if m.diffLines != nil {
		t.Fatal("old diff published")
	}
	execute(t, m, next)
	view := m.View()
	if !strings.Contains(view, "+after file-002.txt") || strings.Contains(view, "+after file-000.txt") {
		t.Fatal(view)
	}
}

func TestReloadGenerationVersionAndPathSafety(t *testing.T) {
	m, f := modelFixture(t, 2)
	first := press(m, "enter")
	newState := changes.View{Generation: 2, Version: 8, Changes: []changes.Summary{{Path: "file-000.txt", Kind: changes.Modified, Version: 8}}}
	m.acceptView(newState)
	m.acceptView(changes.View{Generation: 1, Version: 999, Changes: f.state.Changes})
	if m.state.Generation != 2 || m.state.Version != 8 {
		t.Fatal("old view rolled back")
	}
	next := execute(t, m, first)
	if next == nil {
		t.Fatal("latest diff not scheduled")
	}
	f.diffFn = func(context.Context, string) (diff.Result, error) {
		return diff.Result{Path: "wrong-file", Generation: 2, Version: 8, Status: diff.Text}, nil
	}
	retry := execute(t, m, next)
	if !m.retryPending || retry == nil || !strings.Contains(m.View(), "Diff unavailable") {
		t.Fatal("wrong identity displayed", m.View())
	}
	m.acceptView(changes.View{Generation: 3, Version: 0})
	if len(m.visible) != 0 || m.diffLines != nil {
		t.Fatal("reset retained old content")
	}
}

func TestStaleDiffRetriesAreBoundedAndActuallyRun(t *testing.T) {
	m, f := modelFixture(t, 1)
	calls := 0
	f.diffFn = func(context.Context, string) (diff.Result, error) { calls++; return diff.Result{}, changes.ErrStale }
	cmd := press(m, "enter")
	for attempt := 0; attempt < 4; attempt++ {
		next := execute(t, m, cmd)
		if attempt == 3 {
			if next != nil || m.retryPending {
				t.Fatal("unbounded retry")
			}
			break
		}
		if next == nil {
			t.Fatal("missing bounded retry")
		}
		_, cmd = m.Update(retryMsg{epoch: m.epoch})
		if cmd == nil {
			t.Fatal("retry blocked by displayed error")
		}
	}
	if calls != 4 {
		t.Fatal(calls)
	}
}

func TestSessionControlsConfirmationWarningsAndFatal(t *testing.T) {
	m, f := modelFixture(t, 1)
	cmd := press(m, "p")
	if press(m, "p") != nil {
		t.Fatal("duplicate control")
	}
	execute(t, m, cmd)
	if m.status != app.Paused {
		t.Fatal(m.status)
	}
	execute(t, m, press(m, "p"))
	if m.status != app.Monitoring {
		t.Fatal(m.status)
	}
	press(m, "r")
	if !m.confirmReset {
		t.Fatal("missing confirmation")
	}
	press(m, "n")
	if len(f.actions) != 2 || len(m.visible) != 1 {
		t.Fatal("cancel reset changed data")
	}
	f.controlErr = errors.New("permission denied")
	press(m, "r")
	execute(t, m, press(m, "y"))
	if len(m.visible) != 1 || !strings.Contains(m.notice, "retained") {
		t.Fatal(m.notice)
	}
	f.controlErr = nil
	press(m, "r")
	execute(t, m, press(m, "enter"))
	if m.state.Generation != 2 || len(m.visible) != 0 {
		t.Fatal(m.state)
	}
	m.Update(eventMsg{event: app.Event{Type: "warning", Message: "cannot read one file"}, state: f.state, status: app.Monitoring})
	if m.err != nil || !strings.Contains(m.View(), "monitoring continues") {
		t.Fatal(m.View())
	}
	press(m, "e")
	if !strings.Contains(m.View(), "permission denied") {
		t.Fatal(m.View())
	}
	_, quit := m.Update(stoppedMsg{err: errors.New("root gone")})
	if m.err == nil || quit == nil {
		t.Fatal("fatal did not quit")
	}
}

func TestMouseSelectionAndScrolling(t *testing.T) {
	m, _ := modelFixture(t, 120)
	_, cmd := m.Update(tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.selected != 1 || !m.expanded {
		t.Fatal("mouse selection")
	}
	execute(t, m, cmd)
	m.Update(tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.expanded {
		t.Fatal("mouse collapse")
	}
	m.Update(tea.MouseMsg{X: 5, Y: 4, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.selected != 4 {
		t.Fatal("list wheel", m.selected)
	}
	before := m.selected
	m.Update(tea.MouseMsg{X: 5, Y: -10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.selected != before {
		t.Fatal("outside click")
	}
}

func TestEventWaitCancellationAndClose(t *testing.T) {
	_, f := modelFixture(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := waitEvent(ctx, f)().(stoppedMsg); !ok {
		t.Fatal("wait cancel")
	}
	close(f.events)
	if _, ok := waitEvent(context.Background(), f)().(stoppedMsg); !ok {
		t.Fatal("wait closed")
	}
}

func BenchmarkVisibleDiffRendering(b *testing.B) {
	m := newModel(context.Background(), "/project", nil, io.Discard)
	m.palette.plain = true
	m.width, m.height = 120, 40
	m.expanded = true
	for i := 0; i < 1500; i++ {
		m.diffLines = append(m.diffLines, viewLine{text: fmt.Sprintf("%6d +line", i), style: "added"})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
