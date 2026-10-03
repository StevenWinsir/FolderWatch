package backend

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/app"
)

// API is the ONLY object bound to Wails. Lifecycle hooks and the core remain
// unreachable from JavaScript; every RPC has an explicit DTO response envelope.
type API struct {
	f            *Facade
	selectFolder func() (string, error)
}

func NewAPI(f *Facade, picker ...func() (string, error)) *API {
	a := &API{f: f}
	if len(picker) > 0 {
		a.selectFolder = picker[0]
	}
	return a
}
func (a *API) GetAppInfo() AppInfo { return a.f.info }

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
		view := r.session.ChangeState()
		f.headLocked(view.Generation, view.Version)
	}
	return Reply{Status: f.status}
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
func (a *API) control(req SessionRequest, action string) Reply {
	r, s, err := a.session(req)
	if err != nil {
		return Reply{Status: idle(), Error: problem(err)}
	}
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
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
	view := s.ChangeState()
	f.headLocked(view.Generation, view.Version)
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
