// Package progress carries optional metadata-only, throttled operation progress.
// It owns no goroutine or timer; callers report from their existing work owner.
package progress

import (
	"context"
	"time"
)

type key struct{}
type Update struct {
	Operation string
	Processed uint64
}
type Callback func(Update)

func WithCallback(ctx context.Context, callback Callback) context.Context {
	return context.WithValue(ctx, key{}, callback)
}

type Reporter struct {
	callback Callback
	update   Update
	last     time.Time
}

func Start(ctx context.Context, operation string) *Reporter {
	callback, _ := ctx.Value(key{}).(Callback)
	r := &Reporter{callback: callback, update: Update{Operation: operation}, last: time.Now()}
	if callback != nil {
		callback(r.update)
	}
	return r
}
func (r *Reporter) Step() {
	r.update.Processed++
	if r.callback != nil && time.Since(r.last) >= 250*time.Millisecond {
		r.last = time.Now()
		r.callback(r.update)
	}
}
func (r *Reporter) Finish() {
	if r.callback != nil {
		r.callback(Update{})
	}
}
