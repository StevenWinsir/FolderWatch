package app

import "context"

// Status is UI-independent. Paused freezes semantic resolution, not the watcher.
type Status string

const (
	Idle       Status = "Idle"
	Scanning   Status = "Scanning"
	Monitoring Status = "Monitoring"
	Paused     Status = "Paused"
	Stopping   Status = "Stopping"
	Error      Status = "Error"
)

func (s *Session) Status() Status          { s.mu.Lock(); defer s.mu.Unlock(); return s.status }
func (s *Session) setStatus(status Status) { s.mu.Lock(); s.status = status; s.mu.Unlock() }

// Pause is a serialization barrier: after success the last-known ChangeStore
// stays fixed until Resume or an explicit Reset. Fatal watcher errors still stop.
func (s *Session) Pause(ctx context.Context) error { return s.control(ctx, "pause") }

// Resume reconciles the entire root against the same baseline before returning.
// Cancellation retains Paused; recoverable paths retain last-known state and retry.
func (s *Session) Resume(ctx context.Context) error { return s.control(ctx, "resume") }
