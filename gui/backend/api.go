package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/config"
)

// API is the ONLY object bound to Wails. Lifecycle hooks and the core remain
// unreachable from JavaScript; every RPC has an explicit DTO response envelope.
type API struct {
	f            *Facade
	selectFolder func() (string, error)
	system       systemActions
}

func NewAPI(f *Facade, picker ...func() (string, error)) *API {
	a := &API{f: f, system: defaultSystemActions()}
	if len(picker) > 0 {
		a.selectFolder = picker[0]
	}
	return a
}

// setSystemActions is intentionally package-private: production uses the
// native defaults while backend tests inject deterministic recorders.
func (a *API) setSystemActions(actions systemActions) { a.system = actions }
func (a *API) GetAppInfo() AppInfo                    { return a.f.info }

func guiSettings(cfg config.Config) GUISettings {
	ignore := append([]string{}, cfg.Ignore...)
	return GUISettings{
		Debounce:         cfg.Debounce.String(),
		Ignore:           ignore,
		RespectGitIgnore: cfg.RespectGitIgnore,
		MaxDiffBytes:     strconv.FormatInt(cfg.MaxDiffBytes, 10),
		Editor:           cfg.Editor,
	}
}

// GetSettings reads the same user/project configuration used by CLI startup.
// It performs no scan and therefore is safe to call while the user edits the
// settings form or before a session exists.
func (a *API) GetSettings(req SettingsRequest) SettingsReply {
	out := SettingsReply{Settings: guiSettings(config.Defaults())}
	a.f.mu.Lock()
	if err := a.f.authLocked(req.ClientID); err != nil {
		a.f.mu.Unlock()
		out.Error = problem(err)
		return out
	}
	a.f.mu.Unlock()
	if req.Root == "" {
		return out
	}
	if len(req.Root) > 32768 {
		out.Error = problem(fault("INVALID_ROOT", "Choose an existing directory using an absolute path or ~/path."))
		return out
	}
	cfg, err := config.Load(req.Root, config.Overlay{}, config.LoadOptions{})
	if err != nil {
		out.Error = problem(fmt.Errorf("settings: %w", err))
		return out
	}
	out.Settings = guiSettings(cfg)
	return out
}

func (a *API) SelectFolder() FolderReply {
	if a.selectFolder == nil {
		return FolderReply{Error: problem(errors.New("native folder picker is unavailable"))}
	}
	path, err := a.selectFolder()
	return FolderReply{Path: path, Error: problem(err)}
}

func (a *API) AttachFrontend() ConnectionReply {
	client, state, err := a.f.attach()
	return ConnectionReply{ClientID: client, App: a.f.info, Status: state, Error: problem(err)}
}
func (a *API) DetachFrontend(clientID string) Reply {
	err := a.f.detach(clientID)
	return Reply{Status: idle(), Error: problem(err)}
}
func (a *API) Heartbeat(clientID string) Reply {
	f := a.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.authLocked(clientID); err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	f.expires = time.Now().Add(f.options.lease)
	return Reply{Status: f.status}
}
func (a *API) StartSession(opts StartOptions) Reply {
	state, err := a.f.start(opts)
	return Reply{Status: state, Error: problem(err)}
}
func (a *API) GetSessionStatus(clientID string) Reply {
	f := a.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.authLocked(clientID); err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	if r := f.current; r != nil && r.session != nil && r.ctx.Err() == nil {
		f.status.State = string(r.session.Status())
		generation, version := r.session.ChangeHead()
		f.headLocked(generation, version)
	}
	return Reply{Status: f.status}
}

func (a *API) statusSnapshot(clientID string) SessionInfo {
	return a.GetSessionStatus(clientID).Status
}

func (a *API) StopSession(req SessionRequest) Reply {
	f := a.f
	f.mu.Lock()
	if err := f.authLocked(req.ClientID); err != nil {
		f.mu.Unlock()
		return Reply{Status: idle(), Error: problem(err)}
	}
	if req.SessionID != "" && req.SessionID == f.status.SessionID && f.current == nil {
		state := f.status
		f.mu.Unlock()
		return Reply{Status: state}
	}
	// Stop is also permitted while startup or another Stop is in progress.
	r := f.current
	if r == nil || req.SessionID == "" || req.SessionID != r.id || req.ClientID != r.client {
		f.mu.Unlock()
		return Reply{Status: idle(), Error: problem(fault("STALE_SESSION", "This request does not belong to the active session."))}
	}
	f.status.State = "Stopping"
	f.publishLocked(EventStatus, nil)
	r.cancel()
	f.mu.Unlock()
	<-r.done
	return a.GetSessionStatus(req.ClientID)
}

func (a *API) PauseSession(req SessionRequest) Reply  { return a.control(req, "pause") }
func (a *API) ResumeSession(req SessionRequest) Reply { return a.control(req, "resume") }
func (a *API) ResetBaseline(req SessionRequest) Reply { return a.control(req, "reset") }

func (a *API) pathFor(req SessionRequest, key string) (*run, string, error) {
	r, _, err := a.session(req)
	if err != nil {
		return nil, "", err
	}
	if err := safeKey(r.root, key); err != nil {
		return nil, "", err
	}
	return r, filepath.Join(r.root, filepath.FromSlash(key)), nil
}

func (a *API) OpenInEditor(req EditorRequest) Reply {
	r, absolute, err := a.pathFor(req.SessionRequest, req.Path)
	if err == nil {
		editor := req.Editor
		if editor == "" {
			editor = r.editor
		}
		err = a.system.openEditor(context.Background(), editor, absolute)
	}
	return Reply{Status: a.statusSnapshot(req.ClientID), Error: problem(err)}
}

func (a *API) RevealInFinder(req PathRequest) Reply {
	_, absolute, err := a.pathFor(req.SessionRequest, req.Path)
	if err == nil {
		err = a.system.reveal(context.Background(), absolute)
	}
	return Reply{Status: a.statusSnapshot(req.ClientID), Error: problem(err)}
}

func (a *API) CopyPath(req CopyPathRequest) Reply {
	_, absolute, err := a.pathFor(req.SessionRequest, req.Path)
	value := absolute
	if req.Relative {
		value = req.Path
	}
	if err == nil {
		err = a.system.copy(context.Background(), value)
	}
	return Reply{Status: a.statusSnapshot(req.ClientID), Error: problem(err)}
}
func (a *API) control(req SessionRequest, action string) Reply {
	r, s, err := a.session(req)
	if err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	// Large resets/recovery are cancellable work, not arbitrary 30-second
	// deadlines. Stop/reload cancels r.ctx; reject duplicate controls promptly.
	select {
	case a.f.controlSlot <- struct{}{}:
		defer func() { <-a.f.controlSlot }()
	default:
		return Reply{Status: a.statusSnapshot(req.ClientID), Error: problem(fault("BUSY", "A session operation is already running. Stop cancels it."))}
	}
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	switch action {
	case "pause":
		err = s.Pause(ctx)
	case "resume":
		err = s.Resume(ctx)
	case "reset":
		err = s.ResetBaseline(ctx)
	}
	if err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	f := a.f
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := f.sessionLocked(req); err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	generation, version := s.ChangeHead()
	f.headLocked(generation, version)
	if action == "reset" {
		f.status.Warning = ""
	}
	f.status.State = string(s.Status())
	f.publishLocked(EventStatus, nil)
	return Reply{Status: f.status}
}

func (a *API) session(req SessionRequest) (*run, *app.Session, error) {
	f := a.f
	f.mu.Lock()
	defer f.mu.Unlock()
	r, err := f.sessionLocked(req)
	if err != nil {
		return nil, nil, err
	}
	if r.session == nil {
		return nil, nil, fault("BUSY", "The initial baseline is still being scanned.")
	}
	return r, r.session, nil
}

func validCounter(s string) bool {
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && decimal(n) == s
}
