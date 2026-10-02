package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/logging"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is owned solely by Bubble Tea's Update loop. Commands capture immutable
// arguments, never read mutable model fields, and never write the terminal.
type Model struct {
	noMouse                                              bool
	ctx                                                  context.Context
	root                                                 string
	service                                              backend
	start                                                tea.Cmd
	log                                                  *logging.Logger
	palette                                              palette
	state                                                changes.View
	visible                                              []changes.Summary
	selected, listTop                                    int
	width, height                                        int
	status                                               app.Status
	filter, savedFilter                                  string
	filtering, expanded, confirmReset, help, diagnostics bool
	overlayTop                                           int
	diagnosticLines                                      []string
	busy, notice                                         string
	err                                                  error
	interrupted                                          bool
	epoch                                                uint64
	diffRunning, retryPending                            bool
	diffCancel                                           context.CancelFunc
	diffLines                                            []viewLine
	diffTop, diffLeft, retries                           int
}

func newModel(ctx context.Context, root string, log *logging.Logger, output io.Writer) *Model {
	return &Model{ctx: ctx, root: root, log: log, status: app.Scanning, palette: newPalette(output), notice: "Building a startup baseline; q cancels safely."}
}
func (m *Model) Init() tea.Cmd {
	if m.noMouse {
		return tea.Batch(m.start, tea.DisableMouse)
	}
	return m.start
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case startedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.service = msg.service
		m.status = msg.service.Status()
		m.acceptView(msg.service.ChangeState())
		m.notice = "Monitoring changes relative to the startup baseline."
		m.log.Record("info", "session ready")
		return m, waitEvent(m.ctx, m.service)
	case stoppedMsg:
		m.invalidateDiff()
		m.err = msg.err
		if msg.err != nil {
			m.log.Record("error", msg.err.Error())
		}
		return m, tea.Quit
	case eventMsg:
		// Event and control commands can finish out of order. Status has no
		// ChangeState version, so consult the core's short-lock accessor rather
		// than letting a captured pre-Pause snapshot roll the controls back.
		m.status = m.service.Status()
		m.acceptView(msg.state)
		m.log.Record("debug", fmt.Sprintf("event=%s generation=%d version=%d changed=%d", msg.event.Type, msg.state.Generation, msg.state.Version, len(msg.state.Changes)))
		if msg.event.Type == "warning" {
			m.notice = "Warning: " + msg.event.Message + "; monitoring continues (e: details)."
			m.log.Record("warning", msg.event.Message)
		}
		if m.log.Err() != nil {
			m.notice = "Log file stopped; monitoring continues. Press e for details."
		}
		if m.diagnostics {
			m.loadDiagnostics()
		}
		return m, tea.Batch(waitEvent(m.ctx, m.service), m.requestDiff())
	case controlMsg:
		m.busy = ""
		m.status = m.service.Status()
		m.acceptView(msg.state)
		if msg.err != nil {
			m.notice = msg.action + " failed: " + msg.err.Error() + "; previous baseline retained (e: details)."
			m.log.Record("warning", m.notice)
		} else {
			m.notice = map[string]string{"pause": "Paused: list is last-known; p reconciles and resumes.", "resume": "Monitoring: reconciled with the same baseline.", "reset": "Baseline reset: current files are the new comparison point."}[msg.action]
			m.log.Record("info", m.notice)
		}
		return m, m.requestDiff()
	case diffMsg:
		m.diffRunning = false
		if m.diffCancel != nil {
			m.diffCancel()
			m.diffCancel = nil
		}
		if msg.epoch != m.epoch || !m.expanded {
			return m, m.requestDiff()
		}
		selected, ok := m.current()
		if !ok {
			return m, nil
		}
		if msg.err == nil && (msg.result.Path != selected.Path || msg.result.Generation != m.state.Generation || msg.result.Version != selected.Version) {
			msg.err = changes.ErrStale
		}
		if msg.err != nil {
			m.diffLines = []viewLine{{text: "Diff unavailable: " + safeText(msg.err.Error()) + ". Enter twice to retry."}}
			if (errors.Is(msg.err, changes.ErrStale) || errors.Is(msg.err, changes.ErrNotChanged)) && m.retries < 3 {
				m.retries++
				m.retryPending = true
				epoch := m.epoch
				return m, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return retryMsg{epoch} })
			}
			if !errors.Is(msg.err, context.Canceled) {
				m.log.Record("warning", "diff: "+msg.err.Error())
			}
			return m, nil
		}
		m.diffLines = msg.lines
		m.clamp()
		return m, nil
	case retryMsg:
		if msg.epoch != m.epoch {
			return m, nil
		}
		m.retryPending = false
		m.diffLines = nil
		return m, m.requestDiff()
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		m.clamp()
	case tea.KeyMsg:
		return m, m.key(msg)
	case tea.MouseMsg:
		return m, m.mouse(tea.MouseEvent(msg))
	}
	return m, nil
}

func (m *Model) current() (changes.Summary, bool) {
	if m.selected < 0 || m.selected >= len(m.visible) {
		return changes.Summary{}, false
	}
	return m.visible[m.selected], true
}
func (m *Model) acceptView(view changes.View) {
	if view.Generation < m.state.Generation || (view.Generation == m.state.Generation && view.Version < m.state.Version) {
		return
	}
	old, _ := m.current()
	generation := m.state.Generation
	m.state = view
	m.refilter(old.Path)
	current, ok := m.current()
	if !ok || generation != view.Generation || old.Path != current.Path || old.Version != current.Version {
		m.invalidateDiff()
	}
}
func (m *Model) refilter(preferred string) {
	m.visible = nil
	query := strings.ToLower(m.filter)
	for _, item := range m.state.Changes {
		if strings.Contains(strings.ToLower(item.Path), query) {
			m.visible = append(m.visible, item)
		}
	}
	for i, item := range m.visible {
		if item.Path == preferred {
			m.selected = i
			break
		}
	}
	m.clamp()
}
func (m *Model) invalidateDiff() {
	m.epoch++
	if m.diffCancel != nil {
		m.diffCancel()
	}
	m.diffLines = nil
	m.diffTop, m.diffLeft, m.retries = 0, 0, 0
	m.retryPending = false
}
func (m *Model) requestDiff() tea.Cmd {
	if m.service == nil || !m.expanded || m.diffRunning || m.retryPending || m.busy != "" || m.diffLines != nil {
		return nil
	}
	item, ok := m.current()
	if !ok {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.diffCancel, m.diffRunning = cancel, true
	epoch, service := m.epoch, m.service
	return func() tea.Msg {
		result, err := service.GetDiff(ctx, item.Path)
		var lines []viewLine
		if err == nil {
			lines, err = formatDiff(ctx, result, item)
		}
		return diffMsg{epoch: epoch, result: result, lines: lines, err: err}
	}
}
func (m *Model) move(delta int) tea.Cmd {
	previous := m.selected
	m.selected = max(0, min(len(m.visible)-1, m.selected+delta))
	m.clamp()
	if previous != m.selected {
		m.invalidateDiff()
	}
	return m.requestDiff()
}
func (m *Model) toggle() tea.Cmd {
	if _, ok := m.current(); !ok {
		return nil
	}
	m.expanded = !m.expanded
	m.invalidateDiff()
	return m.requestDiff()
}
func (m *Model) control(action string) tea.Cmd {
	if m.service == nil || m.busy != "" {
		return nil
	}
	m.busy = action
	m.notice = action + " in progress; q still exits safely."
	m.invalidateDiff()
	ctx, service := m.ctx, m.service
	return func() tea.Msg {
		var err error
		switch action {
		case "pause":
			err = service.Pause(ctx)
		case "resume":
			err = service.Resume(ctx)
		case "reset":
			err = service.ResetBaseline(ctx)
		}
		return controlMsg{action: action, err: err, state: service.ChangeState(), status: service.Status()}
	}
}
func (m *Model) key(key tea.KeyMsg) tea.Cmd {
	name := key.String()
	if name == "ctrl+c" {
		m.interrupted = true
		m.invalidateDiff()
		return tea.Quit
	}
	if m.filtering {
		switch name {
		case "esc":
			m.filter = m.savedFilter
			m.filtering = false
		case "enter":
			m.filtering = false
		case "backspace", "ctrl+h":
			r := []rune(m.filter)
			if len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
		default:
			if key.Type == tea.KeyRunes && utf8.RuneCountInString(m.filter)+len(key.Runes) <= 256 {
				m.filter += string(key.Runes)
			}
			if key.Type == tea.KeySpace && utf8.RuneCountInString(m.filter) < 256 {
				m.filter += " "
			}
		}
		m.selected, m.listTop = 0, 0
		m.refilter("")
		m.invalidateDiff()
		return m.requestDiff()
	}
	if name == "q" {
		m.invalidateDiff()
		return tea.Quit
	}
	if m.confirmReset {
		m.confirmReset = false
		if name == "y" || name == "enter" {
			return m.control("reset")
		}
		m.notice = "Reset cancelled; baseline unchanged."
		return nil
	}
	if m.help || m.diagnostics {
		switch name {
		case "esc", "?", "e":
			m.help, m.diagnostics = false, false
		case "down", "j":
			m.overlayTop++
		case "up", "k":
			m.overlayTop = max(0, m.overlayTop-1)
		case "pgdown", "ctrl+d":
			m.overlayTop += max(1, m.height-5)
		case "pgup", "ctrl+u":
			m.overlayTop = max(0, m.overlayTop-max(1, m.height-5))
		}
		m.clamp()
		return nil
	}
	switch name {
	case "down", "j":
		return m.move(1)
	case "up", "k":
		return m.move(-1)
	case "enter", " ":
		return m.toggle()
	case "p":
		if m.status == app.Paused {
			return m.control("resume")
		}
		return m.control("pause")
	case "r":
		if m.service != nil && m.busy == "" {
			m.confirmReset = true
			m.notice = "Reset baseline to current files and clear changes? y/Enter confirm; any other key cancels."
		}
	case "/":
		m.savedFilter = m.filter
		m.filtering = true
	case "esc":
		m.filter = ""
		m.refilter("")
		m.invalidateDiff()
		return m.requestDiff()
	case "?":
		m.help = true
		m.overlayTop = 0
	case "e":
		m.diagnostics = true
		m.overlayTop = 0
		m.loadDiagnostics()
	case "pgdown", "ctrl+d":
		if m.expanded {
			m.diffTop += m.diffHeight()
		} else {
			return m.move(m.listHeight())
		}
	case "pgup", "ctrl+u":
		if m.expanded {
			m.diffTop -= m.diffHeight()
		} else {
			return m.move(-m.listHeight())
		}
	case "home", "g":
		if m.expanded {
			m.diffTop = 0
		} else {
			return m.move(-len(m.visible))
		}
	case "end", "G":
		if m.expanded {
			m.diffTop = len(m.diffLines)
		} else {
			return m.move(len(m.visible))
		}
	case "right", "l":
		m.diffLeft = min(16384, m.diffLeft+8)
	case "left", "h":
		m.diffLeft = max(0, m.diffLeft-8)
	}
	m.clamp()
	return nil
}
func (m *Model) mouse(event tea.MouseEvent) tea.Cmd {
	if m.noMouse {
		return nil
	}
	if m.help || m.diagnostics || m.filtering || m.confirmReset || m.width < 40 || m.height < 12 {
		return nil
	}
	if event.Button == tea.MouseButtonWheelUp || event.Button == tea.MouseButtonWheelDown {
		delta := 3
		if event.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		if event.Y >= m.diffStart() {
			m.diffTop += delta
			m.clamp()
			return nil
		}
		return m.move(delta)
	}
	if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress && event.X >= 0 && event.X < m.width && event.Y >= 3 && event.Y < 3+m.listHeight() {
		index := m.listTop + event.Y - 3
		if index >= len(m.visible) {
			return nil
		}
		if m.selected == index {
			return m.toggle()
		}
		m.selected = index
		m.expanded = true
		m.invalidateDiff()
		m.clamp()
		return m.requestDiff()
	}
	return nil
}
func (m *Model) listHeight() int { return max(1, min(12, (m.height-7)/3)) }
func (m *Model) diffStart() int  { return 4 + m.listHeight() }
func (m *Model) diffHeight() int { return max(1, m.height-m.diffStart()-2) }
func (m *Model) clamp() {
	m.selected = max(0, min(m.selected, len(m.visible)-1))
	m.listTop = max(0, min(m.listTop, len(m.visible)-m.listHeight()))
	if m.selected < m.listTop {
		m.listTop = m.selected
	}
	if m.selected >= m.listTop+m.listHeight() {
		m.listTop = m.selected - m.listHeight() + 1
	}
	m.diffTop = max(0, min(m.diffTop, len(m.diffLines)-m.diffHeight()))
	count := len(helpLines)
	if m.diagnostics {
		count = len(m.diagnosticLines)
	}
	m.overlayTop = max(0, min(m.overlayTop, count-max(1, m.height-4)))
}
func (m *Model) loadDiagnostics() {
	m.diagnosticLines = nil
	for _, entry := range m.log.Entries() {
		m.diagnosticLines = append(m.diagnosticLines, entry.Time.Format("15:04:05")+" "+entry.Level+" "+safeText(entry.Message))
	}
	if len(m.diagnosticLines) == 0 {
		m.diagnosticLines = []string{"No warnings or diagnostic events recorded."}
	}
	m.clamp()
}
