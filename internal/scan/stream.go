package scan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

// Stream traverses one scope without materializing the tree or keeping one
// descriptor per depth. The directory work queue is an owned temporary file;
// a directory is read in batches of 128 entries. Ordering is deliberately not
// contractual here: ordered output belongs to the metadata index, not the walk.
// Descendant failures are passed to visit, allowing the caller to preserve its
// previous state. Root failure, cancellation and queue I/O failures are fatal.
func Stream(ctx context.Context, root, scope, temp string, filter ignore.Filter, visit func(model.FileMeta, error) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := pathutil.Key(root, scope)
	if err != nil {
		return err
	}
	if filter == nil {
		return fmt.Errorf("scan requires an ignore filter")
	}
	q, err := os.CreateTemp(temp, "walk-queue-*")
	if err != nil {
		return fmt.Errorf("scan queue: %w", err)
	}
	defer func() { _ = q.Close(); _ = os.Remove(q.Name()) }()
	var readAt, writeAt int64
	push := func(value string) error {
		if len(value) > 1<<20 {
			return fmt.Errorf("scan path is too long")
		}
		buf := make([]byte, 4+len(value))
		binary.LittleEndian.PutUint32(buf, uint32(len(value)))
		copy(buf[4:], value)
		n, e := q.WriteAt(buf, writeAt)
		writeAt += int64(n)
		if e == nil && n != len(buf) {
			e = io.ErrShortWrite
		}
		return e
	}
	pop := func() (string, error) {
		var size [4]byte
		if _, e := q.ReadAt(size[:], readAt); e != nil {
			return "", e
		}
		n := binary.LittleEndian.Uint32(size[:])
		if n == 0 || n > 1<<20 {
			return "", fmt.Errorf("invalid scan queue record")
		}
		buf := make([]byte, n)
		_, e := q.ReadAt(buf, readAt+4)
		readAt += int64(4 + n)
		return string(buf), e
	}
	warn := func(key string, kind model.EntryKind, cause error) error {
		if key == "." {
			return fmt.Errorf("scan root %q: %w", root, cause)
		}
		return visit(model.FileMeta{Path: key, Kind: kind}, cause)
	}
	if err := push(key); err != nil {
		return err
	}
	for readAt < writeAt {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := pop()
		if err != nil {
			return err
		}
		if err := pathutil.CheckParents(root, key); err != nil {
			if e := warn(key, model.Directory, err); e != nil {
				return e
			}
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(key))
		info, err := os.Lstat(path)
		if err != nil {
			if e := warn(key, model.Directory, err); e != nil {
				return e
			}
			continue
		}
		excluded, err := filter.Match(key, info.IsDir())
		if err != nil {
			if e := warn(key, kindOf(info), err); e != nil {
				return e
			}
			continue
		}
		if excluded {
			continue
		}
		if err := visit(metadata(key, info), nil); err != nil {
			return err
		}
		if !info.IsDir() {
			continue
		}
		// Check the final directory too. No directory symlink is ever followed.
		if err := pathutil.CheckParents(root, key+"/_"); err != nil {
			if e := warn(key, model.Directory, err); e != nil {
				return e
			}
			continue
		}
		dir, err := os.Open(path)
		if err != nil {
			if e := warn(key, model.Directory, err); e != nil {
				return e
			}
			continue
		}
		walkErr := func() error {
			defer dir.Close()
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				entries, readErr := dir.ReadDir(128)
				for _, entry := range entries {
					if err := ctx.Err(); err != nil {
						return err
					}
					child := filepath.ToSlash(filepath.Join(key, entry.Name()))
					excluded, err := filter.Match(child, entry.IsDir())
					if err != nil {
						if e := warn(child, model.Other, err); e != nil {
							return e
						}
						continue
					}
					if excluded {
						continue
					}
					if entry.IsDir() {
						if err := push(child); err != nil {
							return err
						}
						continue
					}
					info, err := entry.Info()
					if err != nil {
						if e := warn(child, model.Other, err); e != nil {
							return e
						}
						continue
					}
					if info.IsDir() {
						if err := push(child); err != nil {
							return err
						}
						continue
					}
					if err := visit(metadata(child, info), nil); err != nil {
						return err
					}
				}
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				if readErr != nil {
					return warn(key, model.Directory, readErr)
				}
			}
		}()
		if walkErr != nil {
			return walkErr
		}
	}
	return ctx.Err()
}

func kindOf(info fs.FileInfo) model.EntryKind {
	switch {
	case info.IsDir():
		return model.Directory
	case info.Mode()&fs.ModeSymlink != 0:
		return model.Symlink
	case info.Mode().IsRegular():
		return model.RegularFile
	default:
		return model.Other
	}
}

func metadata(key string, info fs.FileInfo) model.FileMeta {
	return model.FileMeta{Path: key, Kind: kindOf(info), Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime()}
}
