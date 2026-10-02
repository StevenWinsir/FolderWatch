// Package app composes core capabilities, independent of Terminal or GUI.
package app

import (
	"context"

	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/ignore"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
)

// InputError distinguishes startup configuration errors from runtime failures.
type InputError struct{ Err error }

func (e *InputError) Error() string { return e.Err.Error() }
func (e *InputError) Unwrap() error { return e.Err }

// Prepared contains R1 inputs for future watchers, NOT a running session.
type Prepared struct {
	Config    config.Config
	Matcher   *ignore.Matcher
	Inventory scan.Result
}

func Prepare(ctx context.Context, root string, overlay config.Overlay, opts config.LoadOptions) (Prepared, error) {
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	cfg, err := config.Load(root, overlay, opts)
	if err != nil {
		return Prepared{}, &InputError{Err: err}
	}
	matcher, err := ignore.New(ignore.Options{
		Root: cfg.Root, Patterns: cfg.Ignore, IgnoreFile: cfg.IgnoreFile,
		RespectGitIgnore: cfg.RespectGitIgnore, IncludeGit: cfg.IncludeGit,
	})
	if err != nil {
		return Prepared{}, &InputError{Err: err}
	}
	inventory, err := scan.Scan(ctx, cfg.Root, matcher)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Config: cfg, Matcher: matcher, Inventory: inventory}, nil
}
