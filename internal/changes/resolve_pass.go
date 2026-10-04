package changes

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/catalog"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/snapshot"
)

// A pass owns unpublished catalogs. The disk-backed seen/blocked sets keep a
// million-path recovery bounded without losing exact unreadable subtree fences.
type resolvePass struct {
	store       *Store
	ctx         context.Context
	next        *catalog.Store[Summary]
	seen        *catalog.Store[bool]
	blocked     *catalog.Store[bool]
	warnings    []Warning
	batch       Batch
	now         time.Time
	nextVersion uint64
	changed     bool
	committed   bool
	direct      bool
	mutations   []catalog.Entry[Summary]
	seenKeys    map[string]bool
	blockedKeys map[string]bool
	count       int
}

func (s *Store) newPass(ctx context.Context, direct bool) (*resolvePass, error) {
	p := &resolvePass{store: s, ctx: ctx, now: time.Now(), nextVersion: s.version + 1,
		batch: Batch{Generation: s.generation, Version: s.version}}
	if direct {
		p.direct = true
		p.seenKeys, p.blockedKeys = make(map[string]bool), make(map[string]bool)
		p.count = s.items.Len()
		return p, nil
	}
	var err error
	p.next, err = s.items.Clone(ctx)
	if err == nil {
		p.seen, err = catalog.New[bool](s.resolveScratch.CacheDir())
	}
	if err == nil {
		p.blocked, err = catalog.New[bool](s.resolveScratch.CacheDir())
	}
	if err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}

func (p *resolvePass) close() {
	var err error
	if p.next != nil && !p.committed {
		err = errors.Join(err, p.next.Close())
	}
	if p.seen != nil {
		err = errors.Join(err, p.seen.Close())
	}
	if p.blocked != nil {
		err = errors.Join(err, p.blocked.Close())
	}
	if err != nil {
		p.store.mu.Lock()
		p.store.closeErr = errors.Join(p.store.closeErr, err)
		p.store.mu.Unlock()
	}
}

func (p *resolvePass) warn(path string, cause error) error {
	if len(p.warnings) < 64 {
		p.warnings = append(p.warnings, Warning{Path: path, Message: cause.Error()})
	}
	if p.direct {
		p.blockedKeys[path] = true
		return nil
	}
	return p.blocked.Put(path, true)
}

func (p *resolvePass) protected(key string) (bool, error) {
	for {
		if p.direct {
			if p.blockedKeys[key] {
				return true, nil
			}
		} else {
			if _, ok, err := p.blocked.Get(key); ok || err != nil {
				return ok, err
			}
		}
		if key == "." {
			return false, nil
		}
		if at := strings.LastIndexByte(key, '/'); at >= 0 {
			key = key[:at]
		} else {
			key = "."
		}
	}
}

func (p *resolvePass) delta(item *Summary, key string) {
	p.changed = true
	if p.batch.Reload {
		return
	}
	// A large change set is an invalidation, not an unlimited event payload.
	if len(p.batch.Upserts)+len(p.batch.Removed) >= 128 {
		p.batch.Reload = true
		p.batch.Upserts, p.batch.Removed = nil, nil
		return
	}
	if item == nil {
		p.batch.Removed = append(p.batch.Removed, key)
	} else {
		p.batch.Upserts = append(p.batch.Upserts, *item)
	}
}

func (p *resolvePass) resolve(key string, force bool) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	if key == "." {
		return nil
	}
	if p.direct {
		if p.seenKeys[key] {
			return nil
		}
		p.seenKeys[key] = true
	} else {
		if _, ok, err := p.seen.Get(key); ok || err != nil {
			return err
		}
		if err := p.seen.Put(key, true); err != nil {
			return err
		}
	}
	if skip, err := p.protected(key); skip || err != nil {
		return err
	}
	s := p.store
	index := p.next
	if p.direct {
		index = s.items
	}
	old, had, err := index.Get(key)
	if err != nil {
		return err
	}
	beforeRef, baselineExists, err := s.snapshots.Lookup(key)
	if err != nil {
		return err
	}
	known := baselineExists && beforeRef.Meta.Kind != model.Directory
	if !known {
		known, err = s.snapshots.Known(key)
		if err != nil {
			return err
		}
	}
	after, err := s.readCurrent(p.ctx, key, force, beforeRef, old)
	if err != nil {
		if p.ctx.Err() != nil {
			return p.ctx.Err()
		}
		if errors.Is(err, snapshot.ErrStorage) || errors.Is(err, snapshot.ErrCapacity) {
			return err
		}
		return p.warn(key, err)
	}
	var before *FileState
	if baselineExists && beforeRef.Meta.Kind != model.Directory {
		before = state(beforeRef)
	}
	if known && equalContent(before, after) {
		if had {
			if err := p.mutate(key, Summary{}, true, had); err != nil {
				return err
			}
			p.delta(nil, key)
		}
		return nil
	}
	kind := Modified
	if !known {
		kind = Unknown
	} else if before == nil {
		kind = Added
	} else if after == nil {
		kind = Deleted
	}
	if had && old.Kind == kind && equalContent(old.After, after) {
		return nil
	}
	item := Summary{Path: key, Kind: kind, Before: before, After: after,
		FirstSeen: p.now, LastSeen: p.now, Version: p.nextVersion}
	if had {
		item.FirstSeen = old.FirstSeen
	}
	if err := p.mutate(key, item, false, had); err != nil {
		return err
	}
	count := p.count
	if !p.direct {
		count = p.next.Len()
	}
	if s.opts.MaxEntries > 0 && count > s.opts.MaxEntries {
		return ErrCapacity
	}
	p.delta(&item, key)
	return nil
}

func (p *resolvePass) commit() (Batch, error) {
	if err := p.ctx.Err(); err != nil {
		return Batch{}, err
	}
	s := p.store
	generation, _ := s.snapshots.Head()
	s.mu.Lock()
	if err := p.ctx.Err(); err != nil {
		s.mu.Unlock()
		return Batch{}, err
	}
	if generation != s.generation {
		s.mu.Unlock()
		return Batch{}, ErrStale
	}
	var old *catalog.Store[Summary]
	if p.direct {
		if err := s.items.Apply(p.mutations); err != nil {
			s.mu.Unlock()
			return Batch{}, err
		}
	} else {
		old, s.items = s.items, p.next
	}
	p.committed = true
	if p.changed {
		s.version = p.nextVersion
		p.batch.Version = p.nextVersion
	}
	s.mu.Unlock()
	if old != nil {
		if err := old.Close(); err != nil {
			s.mu.Lock()
			s.closeErr = errors.Join(s.closeErr, err)
			s.mu.Unlock()
		}
	}
	sort.Slice(p.batch.Upserts, func(i, j int) bool { return p.batch.Upserts[i].Path < p.batch.Upserts[j].Path })
	sort.Strings(p.batch.Removed)
	return p.batch, nil
}

func (p *resolvePass) mutate(key string, item Summary, remove, existed bool) error {
	if !p.direct {
		if remove {
			return p.next.Delete(key)
		}
		return p.next.Put(key, item)
	}
	if len(p.mutations) >= catalog.PageSize {
		return ErrCapacity
	}
	p.mutations = append(p.mutations, catalog.Entry[Summary]{Key: key, Value: item, Delete: remove})
	if remove && existed {
		p.count--
	} else if !remove && !existed {
		p.count++
	}
	return nil
}
