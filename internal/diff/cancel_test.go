package diff

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// countCancel is a deterministic cancellation injection: it cancels the real
// embedded context only after computation has made many cooperative checks.
type countCancel struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *countCancel) Err() error {
	c.calls++
	if c.calls == 100 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestCancellationInsideBoundedMatrix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	injected := &countCancel{Context: ctx, cancel: cancel}
	a, b := []byte(strings.Repeat("a\n", 1024)), []byte(strings.Repeat("b\n", 1024))
	_, err := (BoundedLCS{}).Diff(injected, a, b, Defaults())
	if !errors.Is(err, context.Canceled) || injected.calls < 100 {
		t.Fatalf("computation did not cancel: %v, checks=%d", err, injected.calls)
	}
}
