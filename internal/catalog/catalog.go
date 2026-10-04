// Package catalog provides private, temporary, disk-backed ordered metadata.
// Values are never file contents. Pages and write batches are bounded; callers
// build unpublished catalogs and swap owners instead of keeping whole-tree maps.
package catalog

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const PageSize = 128

var bucket = []byte("metadata")
var ErrClosed = errors.New("metadata catalog closed")

type Entry[T any] struct {
	Key    string
	Value  T
	Delete bool
}

type Store[T any] struct {
	mu     sync.RWMutex
	db     *bolt.DB
	path   string
	count  int
	closed bool
}

// New uses a unique 0600 file within an already private session directory.
// NoSync is intentional: this is reconstructible session scratch, not a durable
// user database. A crash never makes this file the baseline of a later session.
func New[T any](dir string) (*Store[T], error) {
	f, err := os.CreateTemp(dir, "metadata-*.db")
	if err != nil {
		return nil, fmt.Errorf("create metadata catalog: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	db, err := bolt.Open(name, 0600, &bolt.Options{Timeout: time.Second, NoSync: true, NoGrowSync: true, FreelistType: bolt.FreelistMapType})
	if err != nil {
		_ = os.Remove(name)
		return nil, fmt.Errorf("open metadata catalog: %w", err)
	}
	db.AllocSize = 1 << 20
	if err = db.Update(func(tx *bolt.Tx) error { _, e := tx.CreateBucket(bucket); return e }); err != nil {
		_ = db.Close()
		_ = os.Remove(name)
		return nil, err
	}
	return &Store[T]{db: db, path: name}, nil
}

func encode[T any](value T) ([]byte, error) {
	var out bytes.Buffer
	err := gob.NewEncoder(&out).Encode(value)
	return out.Bytes(), err
}

func decode[T any](raw []byte) (T, error) {
	var value T
	err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&value)
	return value, err
}

func (s *Store[T]) Get(key string) (value T, exists bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return value, false, ErrClosed
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(bucket).Get([]byte(key))
		if raw == nil {
			return nil
		}
		exists = true
		value, err = decode[T](raw)
		return err
	})
	return
}

func (s *Store[T]) Put(key string, value T) error {
	return s.Apply([]Entry[T]{{Key: key, Value: value}})
}

func (s *Store[T]) Delete(key string) error {
	return s.Apply([]Entry[T]{{Key: key, Delete: true}})
}

// Apply atomically commits a bounded group. Large work must use an unpublished
// staging catalog so cancellation never publishes a partially updated view.
func (s *Store[T]) Apply(entries []Entry[T]) error {
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > PageSize {
		return fmt.Errorf("metadata batch exceeds %d entries", PageSize)
	}
	type encoded struct {
		key    string
		raw    []byte
		remove bool
	}
	values := make([]encoded, 0, len(entries))
	for _, e := range entries {
		if e.Key == "" {
			return errors.New("empty metadata key")
		}
		var raw []byte
		var err error
		if !e.Delete {
			raw, err = encode(e.Value)
			if err != nil {
				return err
			}
		}
		values = append(values, encoded{e.Key, raw, e.Delete})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	delta := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for _, e := range values {
			present := b.Get([]byte(e.key)) != nil
			if e.remove {
				if present {
					delta--
				}
				if err := b.Delete([]byte(e.key)); err != nil {
					return err
				}
			} else {
				if !present {
					delta++
				}
				if err := b.Put([]byte(e.key), e.raw); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err == nil {
		s.count += delta
	}
	return err
}

// Page owns its values; neither mmap bytes nor an open read transaction escape.
// Releasing the transaction before callbacks permits cancellation and writers,
// and avoids mmap-growth deadlocks from nested read/write transactions.
func (s *Store[T]) Page(ctx context.Context, after, prefix string) ([]Entry[T], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	entries := make([]Entry[T], 0, PageSize)
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucket).Cursor()
		var k, v []byte
		if after != "" {
			k, v = c.Seek([]byte(after))
			if bytes.Equal(k, []byte(after)) {
				k, v = c.Next()
			}
		} else if prefix != "" {
			k, v = c.Seek([]byte(prefix))
		} else {
			k, v = c.First()
		}
		for ; k != nil && len(entries) < PageSize; k, v = c.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if prefix != "" && !bytes.HasPrefix(k, []byte(prefix)) {
				break
			}
			value, err := decode[T](v)
			if err != nil {
				return fmt.Errorf("read metadata %q: %w", k, err)
			}
			entries = append(entries, Entry[T]{Key: string(k), Value: value})
		}
		return nil
	})
	return entries, err
}

func (s *Store[T]) Walk(ctx context.Context, prefix string, visit func(string, T) error) error {
	after := ""
	for {
		page, err := s.Page(ctx, after, prefix)
		if err != nil {
			return err
		}
		for _, e := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(e.Key, e.Value); err != nil {
				return err
			}
		}
		if len(page) < PageSize {
			return ctx.Err()
		}
		after = page[len(page)-1].Key
	}
}

func (s *Store[T]) Clone(ctx context.Context) (*Store[T], error) {
	dst, err := New[T](filepath.Dir(s.path))
	if err != nil {
		return nil, err
	}
	after := ""
	for {
		page, readErr := s.Page(ctx, after, "")
		if readErr == nil {
			readErr = dst.Apply(page)
		}
		if readErr != nil {
			_ = dst.Close()
			return nil, readErr
		}
		if len(page) < PageSize {
			return dst, nil
		}
		after = page[len(page)-1].Key
	}
}

func (s *Store[T]) Len() int { s.mu.RLock(); defer s.mu.RUnlock(); return s.count }

// Window inspects only keys for filtering/counting and decodes the requested
// bounded page. match must be a pure key predicate, not another database call.
func (s *Store[T]) Window(ctx context.Context, offset, limit int, match func(string) bool) ([]Entry[T], int, error) {
	if offset < 0 || limit < 1 || limit > 500 {
		return nil, 0, fmt.Errorf("invalid metadata page")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, 0, ErrClosed
	}
	out := make([]Entry[T], 0, limit)
	matched := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(bucket).Cursor()
		for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if match != nil && !match(string(key)) {
				continue
			}
			if matched >= offset && len(out) < limit {
				value, err := decode[T](raw)
				if err != nil {
					return err
				}
				out = append(out, Entry[T]{Key: string(key), Value: value})
			}
			matched++
			if match == nil && len(out) == limit {
				matched = s.count
				break
			}
		}
		return nil
	})
	return out, matched, err
}

func (s *Store[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.db.Close()
	return errors.Join(err, os.Remove(s.path))
}
