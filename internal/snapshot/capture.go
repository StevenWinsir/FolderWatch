package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

// textProbe is only a conservative storage eligibility check. R3 still must
// classify safely before rendering. It validates the whole byte stream, with
// up to three bytes carried across chunk boundaries for split UTF-8 runes.
type textProbe struct {
	tail   []byte
	binary bool
}

func (p *textProbe) write(chunk []byte) {
	if p.binary {
		return
	}
	data := append(p.tail, chunk...)
	p.tail = nil
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			p.tail = append([]byte{}, data...)
			return
		}
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n == 1 || r < 32 && r != '\n' && r != '\r' && r != '\t' {
			p.binary = true
			return
		}
		data = data[n:]
	}
}

func (s *Store) capture(ctx context.Context, path string) (stored, error) {
	key, err := pathutil.Key(s.root, path)
	if err != nil {
		return stored{}, err
	}
	if err := pathutil.CheckParents(s.root, key); err != nil {
		return stored{}, err
	}
	absolute := filepath.Join(s.root, filepath.FromSlash(key))
	before, err := os.Lstat(absolute)
	if err != nil {
		return stored{}, err
	}
	s.seq++
	item := stored{ref: Ref{ID: filepath.Base(s.dir) + fmt.Sprintf("-%x", s.seq), Meta: model.FileMeta{Path: key, Size: before.Size(), Mode: before.Mode(), ModTime: before.ModTime(), Kind: model.Other}, Retention: "metadata"}}
	if before.Mode()&os.ModeSymlink != 0 {
		item.ref.Meta.Kind = model.Symlink
		target, err := os.Readlink(absolute)
		if err != nil {
			return stored{}, err
		}
		hash := sha256.Sum256([]byte(target))
		item.ref.Hash = hex.EncodeToString(hash[:])
		return item, nil
	}
	if before.IsDir() {
		item.ref.Meta.Kind = model.Directory
		return item, nil
	}
	if !before.Mode().IsRegular() {
		return item, nil
	}
	item.ref.Meta.Kind = model.RegularFile
	file, err := openRegular(absolute)
	if err != nil {
		return stored{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return stored{}, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return stored{}, ErrUnstable
	}
	retain := before.Size() <= s.opts.MaxFileBytes
	var memory bytes.Buffer
	var disk *os.File
	keep := false
	defer func() {
		if disk != nil {
			_ = disk.Close()
			if !keep {
				_ = os.Remove(disk.Name())
			}
		}
	}()
	s.mu.RLock()
	memoryRoom, diskRoom := s.opts.MemoryBytes-s.memory, s.opts.DiskBytes-s.disk
	s.mu.RUnlock()
	if !retain {
		item.ref.Retention = "size"
	} else if before.Size() <= s.opts.MemoryFileBytes && before.Size() <= memoryRoom {
		item.ref.Retention = "memory"
	} else if before.Size() <= diskRoom {
		disk, err = os.CreateTemp(s.dir, "content-")
		if err != nil {
			return stored{}, err
		}
		item.ref.Retention = "disk"
	} else {
		retain = false
		item.ref.Retention = "budget"
	}
	hash := sha256.New()
	buf := make([]byte, 32<<10)
	var total int64
	var probe textProbe
	for {
		if err := ctx.Err(); err != nil {
			return stored{}, err
		}
		// Read at most the initial size plus one byte: a concurrently growing file
		// cannot turn capture into an unlimited stream.
		want := int64(len(buf))
		remain := before.Size() - total
		if remain < want {
			want = remain + 1
		}
		n, readErr := file.Read(buf[:int(want)])
		if n > 0 {
			total += int64(n)
			if total > before.Size() {
				return stored{}, ErrUnstable
			}
			_, _ = hash.Write(buf[:n])
			probe.write(buf[:n])
			if retain && !probe.binary {
				if disk != nil {
					if _, err := disk.Write(buf[:n]); err != nil {
						return stored{}, err
					}
				} else {
					_, _ = memory.Write(buf[:n])
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return stored{}, readErr
		}
		if n == 0 {
			return stored{}, io.ErrNoProgress
		}
	}
	after, err := file.Stat()
	if err != nil {
		return stored{}, err
	}
	current, err := os.Lstat(absolute)
	if err != nil {
		return stored{}, ErrUnstable
	}
	if total != before.Size() || !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() || !after.ModTime().Equal(current.ModTime()) {
		return stored{}, ErrUnstable
	}
	if err := ctx.Err(); err != nil {
		return stored{}, err
	}
	item.ref.Hash = hex.EncodeToString(hash.Sum(nil))
	if probe.binary || len(probe.tail) > 0 {
		item.ref.Retention = "binary"
		retain = false
	}
	if retain {
		item.ref.HasContent = true
		if disk != nil {
			if err := disk.Close(); err != nil {
				return stored{}, err
			}
			item.file = disk.Name()
			keep = true
		} else {
			item.content = append([]byte{}, memory.Bytes()...)
		}
	}
	return item, nil
}

func readBounded(ctx context.Context, r io.Reader, size int64) ([]byte, error) {
	var out bytes.Buffer
	buf := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := r.Read(buf)
		if int64(out.Len())+int64(n) > size {
			return nil, fmt.Errorf("snapshot exceeds owned size")
		}
		if n > 0 {
			_, _ = out.Write(buf[:n])
		}
		if err == io.EOF {
			return out.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}
