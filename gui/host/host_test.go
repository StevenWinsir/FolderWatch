package host

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/StevenWinsir/FolderWatch/gui/backend"
)

func TestHostPumpsAndJoins(t *testing.T) {
	f := backend.New(context.Background(), backend.AppInfo{})
	events := make(chan backend.Event, 8)
	h := New(f, func(_ context.Context, event backend.Event) { events <- event })
	defer h.Close()
	h.Start(context.Background())
	client := backend.NewAPI(f).AttachFrontend()
	if client.Error != nil {
		t.Fatal(client)
	}
	select {
	case event := <-events:
		if event.ClientID != client.ClientID || event.Name != backend.EventStatus {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("event pump did not deliver")
	}
	h.Close()
	if h.Context() != nil {
		t.Fatal("shutdown exposed native runtime context")
	}
	select {
	case <-h.pumpDone:
	default:
		t.Fatal("shutdown did not join event pump")
	}
}

func TestHostCloseBeforeStartupAndConcurrentCallbacks(t *testing.T) {
	for i := 0; i < 50; i++ {
		f := backend.New(context.Background(), backend.AppInfo{})
		h := New(f, func(context.Context, backend.Event) {})
		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				switch j % 3 {
				case 0:
					h.Start(context.Background())
				case 1:
					h.Close()
				case 2:
					_ = h.Context()
				}
			}(j)
		}
		wg.Wait()
		h.Close()
		h.Start(context.Background())
		if h.Context() != nil {
			t.Fatal("closed host restarted")
		}
		if reply := backend.NewAPI(f).AttachFrontend(); reply.Error == nil || reply.Error.Code != "CLOSED" {
			t.Fatal(reply)
		}
	}
}

func TestHostApplicationContextCancellationClosesFacade(t *testing.T) {
	f := backend.New(context.Background(), backend.AppInfo{})
	h := New(f, func(context.Context, backend.Event) {})
	defer h.Close()
	ctx, cancel := context.WithCancel(context.Background())
	h.Start(ctx)
	api := backend.NewAPI(f)
	client := api.AttachFrontend().ClientID
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		reply := api.Heartbeat(client)
		if reply.Error != nil && reply.Error.Code == "CLOSED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native application context cancellation did not close facade")
		}
		time.Sleep(time.Millisecond)
	}
	h.Close()
}
