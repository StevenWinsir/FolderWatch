// Package host joins the native application's context, facade and event pump.
// It has no Wails dependency so lifecycle races can run in ordinary Go CI.
package host

import (
	"context"
	"sync"

	"github.com/StevenWinsir/FolderWatch/gui/backend"
)

type Host struct {
	mu          sync.Mutex
	facade      *backend.Facade
	emit        func(context.Context, backend.Event)
	ctx         context.Context
	pumpDone    chan struct{}
	stopContext func() bool
	closed      bool
}

func New(facade *backend.Facade, emit func(context.Context, backend.Event)) *Host {
	return &Host{facade: facade, emit: emit}
}

func (h *Host) Start(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.pumpDone != nil || ctx == nil {
		return
	}
	h.ctx, h.pumpDone = ctx, make(chan struct{})
	h.stopContext = context.AfterFunc(ctx, h.Close)
	done := h.pumpDone
	go func() {
		defer close(done)
		for event := range h.facade.Events() {
			if h.Context() != nil {
				h.emit(ctx, event)
			}
		}
	}()
}

// Context is safe for native menu callbacks racing with startup or shutdown.
func (h *Host) Context() context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.ctx == nil || h.ctx.Err() != nil {
		return nil
	}
	return h.ctx
}

// Close also works before Start, after a failed native launch, and concurrently.
// Never hold the host lock while waiting for core cleanup or the event consumer.
func (h *Host) Close() {
	h.mu.Lock()
	h.closed = true
	done, stop := h.pumpDone, h.stopContext
	h.mu.Unlock()
	if stop != nil {
		stop()
	}
	h.facade.Close()
	if done != nil {
		<-done
	}
}
