// Package debounce owns bounded trailing-edge path aggregation with one timer.
package debounce

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/eventnorm"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/watcher"
)

type Batch struct {
	Paths     []eventnorm.Request `json:"paths,omitempty"`
	Reconcile bool                `json:"reconcile,omitempty"`
	Scopes    []string            `json:"scopes,omitempty"`
}

type Options struct {
	Delay      time.Duration
	MaxPending int
}
type pending struct {
	first, last time.Time
	metadata    bool
	subtree     bool
}

// Start returns a single-slot output. Slow consumers collapse work to a root
// reconciliation instead of blocking the watcher or growing a queue. A burst
// settles after Delay, with a 4*Delay maximum wait for continuously busy paths.
func Start(ctx context.Context, root string, filter ignore.Filter, input <-chan watcher.RawEvent, opts Options) (<-chan Batch, error) {
	if opts.Delay <= 0 || opts.Delay > time.Duration(1<<63-1)/4 || opts.MaxPending < 1 || opts.MaxPending > 1<<20 || filter == nil {
		return nil, fmt.Errorf("invalid debounce delay or pending capacity")
	}
	out := make(chan Batch, 1)
	go func() {
		defer close(out)
		interval := opts.Delay / 4
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		if interval > 25*time.Millisecond {
			interval = 25 * time.Millisecond
		}
		timer := time.NewTicker(interval)
		defer timer.Stop()
		paths := make(map[string]pending)
		var dirty pending
		rootDirty := false
		markDirty := func(now time.Time) {
			if !rootDirty {
				dirty.first = now
			}
			dirty.last = now
			rootDirty = true
			clear(paths)
		}
		due := func(p pending, now time.Time) bool {
			return now.Sub(p.last) >= opts.Delay || now.Sub(p.first) >= 4*opts.Delay
		}
		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-input:
				if !ok {
					return
				}
				now := time.Now()
				e, keep, err := eventnorm.Normalize(root, filter, raw)
				if err != nil || e.Reconcile && (e.Path == "" || e.Path == ".") {
					markDirty(now)
					continue
				}
				if !keep {
					continue
				}
				if rootDirty {
					dirty.last = now
					continue
				}
				p, exists := paths[e.Path]
				if !exists && len(paths) >= opts.MaxPending {
					markDirty(now)
					continue
				}
				if !exists {
					p = pending{first: now, metadata: true}
				}
				p.last = now
				p.metadata = p.metadata && e.MetadataOnly
				p.subtree = p.subtree || e.Reconcile
				paths[e.Path] = p
			case now := <-timer.C:
				batch := Batch{}
				if rootDirty {
					if !due(dirty, now) {
						continue
					}
					batch.Reconcile = true
					rootDirty = false
				} else {
					for path, p := range paths {
						if due(p, now) {
							if p.subtree {
								batch.Scopes = append(batch.Scopes, path)
							} else {
								batch.Paths = append(batch.Paths, eventnorm.Request{Path: path, MetadataOnly: p.metadata})
							}
							delete(paths, path)
						}
					}
					if len(batch.Paths) == 0 && len(batch.Scopes) == 0 {
						continue
					}
					sort.Slice(batch.Paths, func(i, j int) bool { return batch.Paths[i].Path < batch.Paths[j].Path })
					sort.Strings(batch.Scopes)
				}
				select {
				case out <- batch:
				default:
					markDirty(now)
				}
			}
		}
	}()
	return out, nil
}
