package backend

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/changes"
	"github.com/StevenWinsir/FolderWatch/internal/config"
)

type startFunc func(context.Context, StartOptions) (*app.Session, string, error)
type options struct {
	lease time.Duration
	tick  time.Duration
	start startFunc
}

type run struct {
	id, client, root, editor string
	ctx                      context.Context
	cancel                   context.CancelFunc
	ready, done              chan struct{}
	session                  *app.Session // protected by Facade.mu
	startErr                 error        // published by closing ready, never subsequently changed
}

// Facade owns one frontend lease and at most one core session (including startup
// and teardown). No Wails dependency: tests exercise the same API used by Wails.
// It never opens monitored files, calculates hashes, or derives change semantics.
type Facade struct {
	mu                            sync.Mutex
	ctx                           context.Context
	cancel                        context.CancelFunc
	done                          chan struct{}
	events                        chan Event
	client                        string
	expires                       time.Time
	current                       *run
	status                        SessionInfo
	sequence, generation, version uint64
	options                       options
	info                          AppInfo
	diffSlot                      chan struct{}
}

func New(parent context.Context, info AppInfo) *Facade {
	return newFacade(parent, info, options{lease: 15 * time.Second, tick: time.Second, start: startCore})
}

func newFacade(parent context.Context, info AppInfo, opts options) *Facade {
	ctx, cancel := context.WithCancel(parent)
	info.Name, info.Protocol = "FolderWatch", ProtocolVersion
	info.HeartbeatMillis, info.LeaseMillis = 2000, int(opts.lease/time.Millisecond)
	f := &Facade{ctx: ctx, cancel: cancel, done: make(chan struct{}), events: make(chan Event, 32), options: opts, info: info, diffSlot: make(chan struct{}, 1)}
	f.status = idle()
	go f.reap()
	return f
}

func idle() SessionInfo {
	return SessionInfo{State: "Idle", Sequence: "0", Generation: "0", Version: "0"}
}
func decimal(n uint64) string { return strconv.FormatUint(n, 10) }
func token() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func startCore(ctx context.Context, opts StartOptions) (*app.Session, string, error) {
	prepared, err := app.Prepare(ctx, opts.Root, config.Overlay{Debounce: opts.Debounce, Ignore: opts.Ignore, RespectGitIgnore: opts.RespectGitIgnore, MaxDiffBytes: opts.MaxDiffBytes, Editor: opts.Editor}, config.LoadOptions{})
	if err != nil {
		return nil, "", err
	}
	if opts.Editor == nil {
		opts.Editor = &prepared.Config.Editor
	}
	s, err := app.StartSession(ctx, prepared)
	return s, prepared.Config.Root, err
}

func (f *Facade) Events() <-chan Event { return f.events }
func (f *Facade) Close()               { f.cancel(); <-f.done }

func (f *Facade) authLocked(client string) error {
	if f.ctx.Err() != nil {
		return fault("CLOSED", "The application is shutting down.")
	}
	if client == "" || client != f.client || !time.Now().Before(f.expires) {
		return fault("STALE_CLIENT", "Frontend connection expired. Reconnect before starting a new session.")
	}
	return nil
}

func (f *Facade) sessionLocked(req SessionRequest) (*run, error) {
	if err := f.authLocked(req.ClientID); err != nil {
		return nil, err
	}
	if req.SessionID == "" || f.current == nil || req.SessionID != f.current.id || req.ClientID != f.current.client {
		return nil, fault("STALE_SESSION", "This request does not belong to the active session.")
	}
	if f.current.ctx.Err() != nil {
		return nil, fault("CANCELLED", "The session is stopping.")
	}
	return f.current, nil
}

func (f *Facade) publishLocked(name string, problem *Problem) {
	f.sequence++
	f.status.Sequence = decimal(f.sequence)
	e := Event{Protocol: ProtocolVersion, Name: name, ClientID: f.client, Status: f.status, Reload: true, Problem: problem}
	select {
	case f.events <- e:
		return
	default:
	}
	// Events are bounded invalidations, not a journal. The latest full status
	// always survives overflow; clients pull a versioned list when ready.
	for {
		select {
		case <-f.events:
		default:
			f.events <- e
			return
		}
	}
}

func (f *Facade) headLocked(generation, version uint64) {
	if generation > f.generation || generation == f.generation && version >= f.version {
		f.generation, f.version = generation, version
		f.status.Generation, f.status.Version = decimal(generation), decimal(version)
	}
}

// New frontend attachment revokes the previous capability before cancellation.
// Delayed pagehide/Stop/Diff calls from the old document cannot touch its successor.
func (f *Facade) attach() (string, SessionInfo, error) {
	id, err := token()
	if err != nil {
		return "", idle(), err
	}
	f.mu.Lock()
	if f.ctx.Err() != nil {
		f.mu.Unlock()
		return "", idle(), fault("CLOSED", "The application is shutting down.")
	}
	f.client, f.expires = id, time.Now().Add(f.options.lease)
	r := f.current
	if r != nil {
		r.cancel()
	}
	f.status, f.generation, f.version = idle(), 0, 0
	f.publishLocked(EventStatus, nil)
	f.mu.Unlock()
	if r != nil {
		<-r.done
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.authLocked(id); err != nil {
		return "", f.status, err
	}
	return id, f.status, nil
}

func (f *Facade) detach(client string) error {
	f.mu.Lock()
	if client == "" || client != f.client {
		f.mu.Unlock()
		return nil
	} // idempotent old pagehide
	f.client = ""
	r := f.current
	if r != nil {
		r.cancel()
	}
	f.status, f.generation, f.version = idle(), 0, 0
	f.publishLocked(EventStatus, nil)
	f.mu.Unlock()
	if r != nil {
		<-r.done
	}
	return nil
}

func (f *Facade) reap() {
	ticker := time.NewTicker(f.options.tick)
	defer ticker.Stop()
	defer close(f.done)
	defer close(f.events)
	for {
		select {
		case <-f.ctx.Done():
			f.mu.Lock()
			f.client = ""
			r := f.current
			if r != nil {
				r.cancel()
			}
			f.mu.Unlock()
			if r != nil {
				<-r.done
			}
			return
		case now := <-ticker.C:
			f.mu.Lock()
			if f.client != "" && !now.Before(f.expires) {
				f.client = ""
				if f.current != nil {
					f.current.cancel()
				}
				f.status, f.generation, f.version = idle(), 0, 0
				f.publishLocked(EventStatus, nil)
			}
			f.mu.Unlock()
		}
	}
}

func (f *Facade) start(opts StartOptions) (SessionInfo, error) {
	// Desktop cwd is incidental. A root must be an explicit absolute/home path.
	if len(opts.Root) > 32768 || strings.ContainsRune(opts.Root, 0) || !(filepath.IsAbs(opts.Root) || strings.HasPrefix(opts.Root, "~/")) {
		return idle(), fault("INVALID_ROOT", "Choose an existing directory using an absolute path or ~/path.")
	}
	if len(opts.Ignore) > 1024 {
		return idle(), fault("INVALID_OPTIONS", "At most 1024 ignore patterns are allowed.")
	}
	for _, pattern := range opts.Ignore {
		if len(pattern) > 4096 {
			return idle(), fault("INVALID_OPTIONS", "An ignore pattern is too long.")
		}
	}
	// Resolve the shared CLI/project editor default once for the run. The
	// actual session still loads and validates the complete config in app.Prepare;
	// this lookup only lets system integration use the same editor afterward.
	if opts.Editor == nil {
		if cfg, cfgErr := config.Load(opts.Root, config.Overlay{}, config.LoadOptions{}); cfgErr == nil {
			opts.Editor = &cfg.Editor
		}
	}
	id, err := token()
	if err != nil {
		return idle(), err
	}
	f.mu.Lock()
	if err := f.authLocked(opts.ClientID); err != nil {
		f.mu.Unlock()
		return idle(), err
	}
	if f.current != nil {
		f.mu.Unlock()
		return idle(), fault("BUSY", "Stop the current session before starting another.")
	}
	ctx, cancel := context.WithCancel(f.ctx)
	editor := ""
	if opts.Editor != nil {
		editor = *opts.Editor
	}
	r := &run{id: id, client: opts.ClientID, editor: editor, ctx: ctx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
	f.current = r
	f.status, f.generation, f.version = idle(), 0, 0
	f.status.State, f.status.SessionID, f.status.Root = "Scanning", id, opts.Root
	f.publishLocked(EventStatus, nil)
	// One owned startup/event goroutine, not one goroutine per filesystem event.
	go f.run(r, opts)
	f.mu.Unlock()
	<-r.ready
	// A failed startup is not complete until its resources have been joined.
	// This also permits immediate retry after an invalid/unreadable root.
	if r.startErr != nil {
		<-r.done
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.authLocked(opts.ClientID); err != nil {
		return f.status, err
	}
	if r.startErr != nil {
		return f.status, r.startErr
	}
	if f.current != r || r.ctx.Err() != nil {
		return f.status, fault("CANCELLED", "Session startup was cancelled.")
	}
	return f.status, nil
}

func (f *Facade) run(r *run, opts StartOptions) {
	var s *app.Session
	var final error
	defer func() {
		if s != nil {
			final = errors.Join(final, s.Close())
		}
		r.cancel()
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.current == r {
			f.current = nil
			if f.client == r.client {
				f.status.State = "Idle"
				if final != nil {
					f.status.State, f.status.Problem = "Error", problem(final)
					f.publishLocked(EventError, f.status.Problem)
				} else {
					f.publishLocked(EventStatus, nil)
				}
			}
		}
		close(r.done)
	}()
	var root string
	var err error
	s, root, err = f.options.start(r.ctx, opts)
	f.mu.Lock()
	if err != nil {
		r.startErr = err
		if r.ctx.Err() == nil {
			final = err
		}
	} else if r.ctx.Err() != nil || f.client != r.client {
		r.startErr = context.Canceled
	} else {
		r.session, r.root = s, root
		f.status.State, f.status.Root = string(s.Status()), root
		view := s.ChangeState()
		f.headLocked(view.Generation, view.Version)
		f.publishLocked(EventStatus, nil)
	}
	close(r.ready)
	f.mu.Unlock()
	if err != nil || r.ctx.Err() != nil {
		return
	}
	for {
		select {
		case <-r.ctx.Done():
			return
		case e, ok := <-s.Events():
			if !ok {
				final = s.Err()
				return
			}
			f.mu.Lock()
			if f.current == r && f.client == r.client && r.ctx.Err() == nil {
				f.status.State = string(s.Status()) // never trust a queued old status snapshot
				if e.Batch != nil {
					f.headLocked(e.Batch.Generation, e.Batch.Version)
				}
				switch {
				case e.Type == "warning":
					f.status.Warning = bounded(e.Message)
					f.publishLocked(EventWarning, &Problem{Code: "CORE_WARNING", Message: f.status.Warning})
				case e.Batch != nil && !e.Batch.Empty():
					f.publishLocked(EventChanges, nil)
				case e.Type != "changes":
					f.publishLocked(EventStatus, nil)
				}
			}
			f.mu.Unlock()
		}
	}
}

type ipcError struct{ code, message string }

func (e *ipcError) Error() string      { return e.message }
func fault(code, message string) error { return &ipcError{code, message} }
func bounded(s string) string {
	if len(s) <= 2048 {
		return s
	}
	s = s[:2045]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "..."
}
func problem(err error) *Problem {
	if err == nil {
		return nil
	}
	var ipc *ipcError
	var input *app.InputError
	code := "CORE_ERROR"
	switch {
	case errors.As(err, &ipc):
		code = ipc.code
	case errors.Is(err, context.Canceled), errors.Is(err, app.ErrSessionClosed), errors.Is(err, changes.ErrClosed):
		code = "CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		code = "TIMEOUT"
	case errors.Is(err, changes.ErrStale):
		code = "STALE_VERSION"
	case errors.Is(err, changes.ErrNotChanged):
		code = "NOT_CHANGED"
	case errors.As(err, &input):
		code = "INVALID_OPTIONS"
	}
	return &Problem{Code: code, Message: bounded(err.Error())}
}
