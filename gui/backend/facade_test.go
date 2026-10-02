package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/config"
)

func testFacade(t *testing.T) (*Facade, *API, string) {
	t.Helper()
	f := newFacade(context.Background(), AppInfo{Version: "test"}, options{lease: time.Minute, tick: time.Millisecond * 10, start: func(ctx context.Context, o StartOptions) (*app.Session, string, error) {
		debounce := "5ms"
		if o.Debounce != nil {
			debounce = *o.Debounce
		}
		p, err := app.Prepare(ctx, o.Root, config.Overlay{Debounce: &debounce, Ignore: o.Ignore, RespectGitIgnore: o.RespectGitIgnore, MaxDiffBytes: o.MaxDiffBytes}, config.LoadOptions{SkipUserConfig: true})
		if err != nil {
			return nil, "", err
		}
		s, err := app.StartSession(ctx, p)
		return s, p.Config.Root, err
	}})
	t.Cleanup(f.Close)
	a := NewAPI(f)
	connection := a.AttachFrontend()
	if connection.Error != nil {
		t.Fatal(connection.Error)
	}
	return f, a, connection.ClientID
}
func mustWrite(t *testing.T, name, text string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("condition did not converge before the deadline")
}
func started(t *testing.T, a *API, client, root string) SessionRequest {
	t.Helper()
	reply := a.StartSession(StartOptions{ClientID: client, Root: root})
	if reply.Error != nil || reply.Status.State != "Monitoring" {
		t.Fatalf("start: %+v", reply)
	}
	return SessionRequest{ClientID: client, SessionID: reply.Status.SessionID}
}
func awaitChanges(t *testing.T, a *API, req SessionRequest, count int) ChangesReply {
	t.Helper()
	var reply ChangesReply
	eventually(t, func() bool {
		reply = a.GetChanges(ChangesRequest{SessionRequest: req})
		return reply.Error == nil && len(reply.Changes) == count
	})
	return reply
}
func awaitDiff(t *testing.T, a *API, req SessionRequest, key string) DiffResult {
	t.Helper()
	var result *DiffResult
	eventually(t, func() bool {
		view := a.GetChanges(ChangesRequest{SessionRequest: req})
		if view.Error != nil {
			t.Fatal(view.Error)
		}
		for _, s := range view.Changes {
			if s.Path != key {
				continue
			}
			r := a.GetDiff(DiffRequest{SessionRequest: req, Path: key, Generation: view.Generation, Version: s.Version})
			if r.Error != nil {
				if r.Error.Code == "STALE_VERSION" {
					return false
				}
				t.Fatalf("diff %s: %+v", key, r.Error)
			}
			result = r.Diff
			return result != nil
		}
		return false
	})
	return *result
}
func requireCode(t *testing.T, p *Problem, code string) {
	t.Helper()
	if p == nil || p.Code != code {
		t.Fatalf("expected %s, got %+v", code, p)
	}
}

func TestStartStopStartAndCoreSemantics(t *testing.T) {
	_, a, client := testFacade(t)
	root := filepath.Join(t.TempDir(), "项目 with spaces 'quote'")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "hello 世界.txt")
	mustWrite(t, file, "before\n")
	for cycle := 0; cycle < 3; cycle++ {
		req := started(t, a, client, root)
		requireCode(t, a.StartSession(StartOptions{ClientID: client, Root: root}).Error, "BUSY")
		mustWrite(t, file, "after\n")
		view := awaitChanges(t, a, req, 1)
		if view.Changes[0].Kind != "modified" {
			t.Fatal(view)
		}
		d := awaitDiff(t, a, req, "hello 世界.txt")
		body, _ := json.Marshal(d)
		if !strings.Contains(string(body), "before") || !strings.Contains(string(body), "after") {
			t.Fatal(string(body))
		}
		mustWrite(t, file, "before\n")
		awaitChanges(t, a, req, 0) // restore means removed from list, never Deleted
		stopped := a.StopSession(req)
		if stopped.Error != nil || stopped.Status.State != "Idle" {
			t.Fatal(stopped)
		}
		if repeat := a.StopSession(req); repeat.Error != nil {
			t.Fatal(repeat)
		}
	}
}

func TestReloadRevokesOldClientAndSession(t *testing.T) {
	f, a, oldClient := testFacade(t)
	root := t.TempDir()
	old := started(t, a, oldClient, root)
	f.mu.Lock()
	previous := f.current
	f.mu.Unlock()
	connection := a.AttachFrontend()
	if connection.Error != nil || connection.ClientID == oldClient || connection.Status.State != "Idle" {
		t.Fatal(connection)
	}
	select {
	case <-previous.done:
	default:
		t.Fatal("attach returned before old owner stopped")
	}
	current := started(t, a, connection.ClientID, root)
	if current.SessionID == old.SessionID {
		t.Fatal("reused session token")
	}
	requireCode(t, a.StopSession(old).Error, "STALE_CLIENT")
	requireCode(t, a.ResetBaseline(old).Error, "STALE_CLIENT")
	requireCode(t, a.GetChanges(ChangesRequest{SessionRequest: old}).Error, "STALE_CLIENT")
	a.DetachFrontend(oldClient) // delayed old pagehide MUST NOT detach successor
	if r := a.GetSessionStatus(connection.ClientID); r.Error != nil || r.Status.State != "Monitoring" {
		t.Fatal(r)
	}
	a.DetachFrontend(connection.ClientID)
	f.mu.Lock()
	active := f.current
	f.mu.Unlock()
	if active != nil {
		t.Fatal("detach leaked session")
	}
}

func TestOldSessionRequestsCannotAffectRestart(t *testing.T) {
	_, a, client := testFacade(t)
	root := t.TempDir()
	old := started(t, a, client, root)
	a.StopSession(old)
	current := started(t, a, client, root)
	for _, reply := range []Reply{a.StopSession(old), a.PauseSession(old), a.ResumeSession(old), a.ResetBaseline(old)} {
		requireCode(t, reply.Error, "STALE_SESSION")
	}
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: old}).Error, "STALE_SESSION")
	if r := a.GetSessionStatus(client); r.Status.SessionID != current.SessionID || r.Status.State != "Monitoring" {
		t.Fatal(r)
	}
}

func TestStopDuringScanningCancelsAndJoins(t *testing.T) {
	entered := make(chan struct{})
	f := newFacade(context.Background(), AppInfo{}, options{lease: time.Minute, tick: time.Second, start: func(ctx context.Context, _ StartOptions) (*app.Session, string, error) {
		close(entered)
		<-ctx.Done()
		return nil, "", ctx.Err()
	}})
	defer f.Close()
	a := NewAPI(f)
	client := a.AttachFrontend().ClientID
	result := make(chan Reply, 1)
	go func() { result <- a.StartSession(StartOptions{ClientID: client, Root: t.TempDir()}) }()
	<-entered
	status := a.GetSessionStatus(client).Status
	if status.State != "Scanning" {
		t.Fatal(status)
	}
	stopped := a.StopSession(SessionRequest{ClientID: client, SessionID: status.SessionID})
	if stopped.Error != nil || stopped.Status.State != "Idle" {
		t.Fatal(stopped)
	}
	requireCode(t, (<-result).Error, "CANCELLED")
	f.mu.Lock()
	active := f.current
	f.mu.Unlock()
	if active != nil {
		t.Fatal("startup owner leaked")
	}
}

func TestLeaseExpiryAndHeartbeat(t *testing.T) {
	f, a, client := testFacade(t)
	req := started(t, a, client, t.TempDir())
	f.mu.Lock()
	r := f.current
	f.expires = time.Now().Add(50 * time.Millisecond)
	f.mu.Unlock()
	if reply := a.Heartbeat(client); reply.Error != nil {
		t.Fatal(reply)
	}
	f.mu.Lock()
	extended := time.Until(f.expires)
	f.mu.Unlock()
	if extended < time.Second {
		t.Fatal("heartbeat failed to renew lease")
	}
	f.mu.Lock()
	f.expires = time.Now().Add(-time.Second)
	f.mu.Unlock()
	requireCode(t, a.Heartbeat(client).Error, "STALE_CLIENT") // no revival after expiry
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		t.Fatal("lease expiry leaked watcher")
	}
	requireCode(t, a.StopSession(req).Error, "STALE_CLIENT")
}

func TestPauseResumeResetReuseCore(t *testing.T) {
	f, a, client := testFacade(t)
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	mustWrite(t, file, "one\n")
	req := started(t, a, client, root)
	paused := a.PauseSession(req)
	if paused.Error != nil || paused.Status.State != "Paused" {
		t.Fatal(paused)
	}
	mustWrite(t, file, "two\n")
	if list := a.GetChanges(ChangesRequest{SessionRequest: req}); list.Error != nil || len(list.Changes) != 0 {
		t.Fatal(list)
	}
	resumed := a.ResumeSession(req)
	if resumed.Error != nil || resumed.Status.State != "Monitoring" {
		t.Fatal(resumed)
	}
	view := awaitChanges(t, a, req, 1)
	f.mu.Lock()
	core := f.current.session.ChangeState()
	f.mu.Unlock()
	if decimal(core.Generation) != view.Generation || decimal(core.Version) != view.Version || core.Changes[0].Path != view.Changes[0].Path {
		t.Fatal("GUI diverged from authoritative core")
	}
	reset := a.ResetBaseline(req)
	if reset.Error != nil || reset.Status.Generation == view.Generation {
		t.Fatal(reset)
	}
	awaitChanges(t, a, req, 0)
	mustWrite(t, file, "three\n")
	d := awaitDiff(t, a, req, "a.txt")
	body, _ := json.Marshal(d)
	if strings.Contains(string(body), "one") || !strings.Contains(string(body), "two") {
		t.Fatal("reset did not move baseline", string(body))
	}
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: "a.txt", Generation: view.Generation, Version: view.Changes[0].Version}).Error, "STALE_VERSION")
}

func TestMetadataEventsAndBoundedBackpressure(t *testing.T) {
	f, a, client := testFacade(t)
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	mustWrite(t, file, "PRIVATE_BASELINE_SENTINEL\n")
	req := started(t, a, client, root)
	last := ""
	for i := 0; i < 40; i++ {
		mustWrite(t, file, "PRIVATE_CURRENT_SENTINEL"+strings.Repeat("x", i)+"\n")
		eventually(t, func() bool {
			view := a.GetChanges(ChangesRequest{SessionRequest: req})
			if view.Error != nil || len(view.Changes) != 1 || view.Version == last {
				return false
			}
			last = view.Version
			return true
		})
	}
	if len(f.events) > 32 {
		t.Fatal("unbounded event queue")
	}
	for len(f.events) > 0 {
		e := <-f.events
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 8192 || strings.Contains(string(b), "PRIVATE_") || strings.Contains(string(b), "hunks") || strings.Contains(string(b), "StorageKey") {
			t.Fatalf("unsafe event: %s", b)
		}
		if e.Protocol != 1 || !e.Reload || !validCounter(e.Status.Sequence) {
			t.Fatal(e)
		}
	}
	list, _ := json.Marshal(a.GetChanges(ChangesRequest{SessionRequest: req}))
	if strings.Contains(string(list), "PRIVATE_") {
		t.Fatal("list leaked content")
	}
	if r := a.StopSession(req); r.Error != nil {
		t.Fatal(r)
	}
}

func TestFatalRootLossAndFailureRecovery(t *testing.T) {
	_, a, client := testFacade(t)
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	started(t, a, client, root)
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		r := a.GetSessionStatus(client)
		return r.Status.State == "Error" && r.Status.Problem != nil
	})
	started(t, a, client, t.TempDir())
}

func TestInvalidStartupAndConcurrentClose(t *testing.T) {
	f, a, client := testFacade(t)
	for _, root := range []string{"", ".", "relative", "a\x00b"} {
		requireCode(t, a.StartSession(StartOptions{ClientID: client, Root: root}).Error, "INVALID_ROOT")
	}
	bad := a.StartSession(StartOptions{ClientID: client, Root: filepath.Join(t.TempDir(), "missing")})
	if bad.Error == nil {
		t.Fatal("missing root accepted")
	}
	f.mu.Lock()
	activeAfterFailure := f.current
	f.mu.Unlock()
	if activeAfterFailure != nil {
		t.Fatal("failed Start returned before cleanup")
	}
	req := started(t, a, client, t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 4 {
			case 0:
				a.StopSession(req)
			case 1:
				a.Heartbeat(client)
			case 2:
				a.GetSessionStatus(client)
			case 3:
				f.Close()
			}
		}(i)
	}
	wg.Wait()
	requireCode(t, a.AttachFrontend().Error, "CLOSED")
}

func TestRepeatedLifecycleGoroutinesReturn(t *testing.T) {
	before := runtime.NumGoroutine()
	f, a, client := testFacade(t)
	root := t.TempDir()
	for i := 0; i < 30; i++ {
		req := started(t, a, client, root)
		if r := a.StopSession(req); r.Error != nil {
			t.Fatal(r)
		}
	}
	f.Close()
	eventually(t, func() bool { return runtime.NumGoroutine() <= before+2 })
}
