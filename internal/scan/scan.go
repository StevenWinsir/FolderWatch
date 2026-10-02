// Package scan discovers metadata without reading or following file contents.
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/model"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

type Warning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Result struct {
	Root     string           `json:"root"`
	Entries  []model.FileMeta `json:"entries"`
	Warnings []Warning        `json:"warnings"`
}

// Scan uses filepath.WalkDir, includes root in Entries, and preserves its
// deterministic lexical traversal. Descendant failures are warnings; root
// failure/cancellation is an error, and any returned inventory is incomplete.
func Scan(ctx context.Context, root string, filter ignore.Filter) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	normalized, err := pathutil.NormalizeRoot(root, ".")
	if err != nil {
		return Result{}, err
	}
	if filter == nil {
		return Result{}, fmt.Errorf("scan requires an ignore filter")
	}
	return scanWithWalker(ctx, normalized, filter, filepath.WalkDir)
}

func scanWithWalker(ctx context.Context, root string, filter ignore.Filter, walk func(string, fs.WalkDirFunc) error) (Result, error) {
	result := Result{Root: root, Entries: []model.FileMeta{}, Warnings: []Warning{}}
	err := walk(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := pathutil.Key(root, path)
		if err != nil {
			return err
		}
		warn := func(err error) error {
			if key == "." {
				return fmt.Errorf("scan root %q: %w", root, err)
			}
			result.Warnings = append(result.Warnings, Warning{Path: key, Message: err.Error()})
			// WalkDir calls a directory again if its ReadDir fails, immediately
			// after the successful preorder visit. Do not offer it for watching.
			last := len(result.Entries) - 1
			if last >= 0 && result.Entries[last].Path == key {
				result.Entries = result.Entries[:last]
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			return warn(walkErr)
		}
		if entry == nil {
			return warn(fmt.Errorf("entry disappeared"))
		}
		ignored, err := filter.Match(key, entry.IsDir())
		if err != nil {
			return warn(err)
		}
		if ignored {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return warn(err)
		}
		kind := model.Other
		switch {
		case info.IsDir():
			kind = model.Directory
		case info.Mode()&fs.ModeSymlink != 0:
			kind = model.Symlink
		case info.Mode().IsRegular():
			kind = model.RegularFile
		}
		result.Entries = append(result.Entries, model.FileMeta{
			Path: key, Kind: kind, Size: info.Size(), Mode: info.Mode(), ModTime: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("initial scan: %w", err)
	}
	// Cancellation may occur during the final callback, with no next visit.
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("initial scan: %w", err)
	}
	return result, nil
}
