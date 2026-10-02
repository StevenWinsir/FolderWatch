// Package tui is a presentation adapter; all filesystem semantics live in core.
package tui

import (
	"context"
	"errors"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/diff"
	"github.com/StevenWinsir/FolderWatch/internal/logging"
	tea "github.com/charmbracelet/bubbletea"
)

type backend interface {
	Events() <-chan app.Event
	ChangeState() changes.View
	GetDiff(context.Context, string) (diff.Result, error)
	Pause(context.Context) error
	Resume(context.Context) error
	ResetBaseline(context.Context) error
	Status() app.Status
	Err() error
	Close() error
}

type startedMsg struct {
	service backend
	err     error
}
type eventMsg struct {
	event  app.Event
	state  changes.View
	status app.Status
}
type stoppedMsg struct{ err error }
type controlMsg struct {
	action string
	err    error
	state  changes.View
	status app.Status
}
type diffMsg struct {
	epoch  uint64
	result diff.Result
	lines  []viewLine
	err    error
}
type retryMsg struct{ epoch uint64 }

// Run owns both startup and shutdown, including quitting while baseline capture
// is in flight. Exactly one startup goroutine is joined before returning.
func Run(parent context.Context, prepared app.Prepared, input io.Reader, output io.Writer, log *logging.Logger) (resultErr error) {
	ctx, cancel := context.WithCancel(parent)
	ready := make(chan struct{})
	var startup startedMsg
	go func() {
		service, err := app.StartSession(ctx, prepared)
		startup.err = err
		if service != nil {
			startup.service = service
		}
		close(ready)
	}()
	defer func() {
		cancel()
		<-ready
		if startup.service != nil {
			resultErr = errors.Join(resultErr, startup.service.Close())
		}
	}()
	m := newModel(ctx, prepared.Config.Root, log, output)
	m.noMouse = prepared.Config.NoMouse
	m.start = func() tea.Msg { <-ready; return startup }
	options := []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithoutSignalHandler(), tea.WithFPS(30)}
	if !prepared.Config.NoMouse {
		options = append(options, tea.WithMouseCellMotion())
	}
	final, err := tea.NewProgram(m, options...).Run()
	if parent.Err() != nil {
		return parent.Err()
	}
	if err != nil {
		return err
	}
	if finished, ok := final.(*Model); ok {
		if finished.interrupted {
			return context.Canceled
		}
		return finished.err
	}
	return nil
}

func waitEvent(ctx context.Context, service backend) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return stoppedMsg{err: ctx.Err()}
		case event, ok := <-service.Events():
			if !ok {
				return stoppedMsg{err: service.Err()}
			}
			// Always take a defensive authoritative view, never infer state from
			// raw flags or paths. Its watermark may be ahead of this notification.
			return eventMsg{event: event, state: service.ChangeState(), status: service.Status()}
		}
	}
}
