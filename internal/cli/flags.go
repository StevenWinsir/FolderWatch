// Package cli adapts arguments and streams to the UI-independent core.
package cli

import (
	"fmt"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/spf13/pflag"
)

type Request struct {
	Root    string
	Overlay config.Overlay
	Help    bool
	Version bool
	JSON    bool
	Scan    bool
}

// Parse does not touch the filesystem, making help/version usable anywhere.
func Parse(args []string) (Request, error) {
	r := Request{Root: "."}
	fs := pflag.NewFlagSet("folderwatch", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SetInterspersed(true)
	var debounce, maxBytes, ignoreFile, editor, logFile string
	var patterns []string
	var respectGit, includeGit, noMouse, debug bool
	fs.BoolVarP(&r.Help, "help", "h", false, "Show help")
	fs.BoolVar(&r.Version, "version", false, "Show version")
	fs.BoolVar(&r.JSON, "json", false, "Print scan JSON")
	fs.BoolVar(&r.Scan, "scan", false, "Scan once and exit")
	fs.StringVar(&debounce, "debounce", "150ms", "Positive duration (reserved until R2)")
	fs.StringArrayVar(&patterns, "ignore", nil, "Ignore glob; repeatable")
	fs.StringVar(&ignoreFile, "ignore-file", "", "Additional ignore file")
	fs.BoolVar(&respectGit, "respect-gitignore", false, "Read root/nested .gitignore")
	fs.BoolVar(&includeGit, "include-git", false, "Disable built-in .git/ ignore")
	fs.BoolVar(&noMouse, "no-mouse", false, "Disable TUI mouse (reserved until R4)")
	fs.StringVar(&maxBytes, "max-diff-bytes", "5MiB", "Positive byte budget (reserved until R3)")
	fs.StringVar(&editor, "editor", "", "Editor command text (stored only)")
	fs.StringVar(&logFile, "log-file", "", "Log destination (validated only)")
	fs.BoolVar(&debug, "debug", false, "Debug setting (reserved until R4)")
	if err := fs.Parse(args); err != nil {
		return Request{}, err
	}
	if r.Help || r.Version {
		return r, nil
	}
	if fs.NArg() > 1 {
		return Request{}, fmt.Errorf("expected at most one directory path; got %d", fs.NArg())
	}
	if fs.NArg() == 1 {
		r.Root = fs.Arg(0)
		if r.Root == "" {
			return Request{}, fmt.Errorf("directory path must not be empty")
		}
	}
	if fs.Changed("debounce") {
		r.Overlay.Debounce = &debounce
	}
	if fs.Changed("max-diff-bytes") {
		r.Overlay.MaxDiffBytes = &maxBytes
	}
	if fs.Changed("ignore") {
		r.Overlay.Ignore = patterns
	}
	if fs.Changed("ignore-file") {
		r.Overlay.IgnoreFile = &ignoreFile
	}
	if fs.Changed("respect-gitignore") {
		r.Overlay.RespectGitIgnore = &respectGit
	}
	if fs.Changed("include-git") {
		r.Overlay.IncludeGit = &includeGit
	}
	if fs.Changed("no-mouse") {
		r.Overlay.NoMouse = &noMouse
	}
	if fs.Changed("editor") {
		r.Overlay.Editor = &editor
	}
	if fs.Changed("log-file") {
		r.Overlay.LogFile = &logFile
	}
	if fs.Changed("debug") {
		r.Overlay.Debug = &debug
	}
	return r, nil
}

const HelpText = `FolderWatch — local folder change inspection

Usage:
  folderwatch [flags] [path]
  folderwatch --scan --json .

R1 (P0–P2): scans metadata once and exits. No live watcher, content baseline,
text diff, TUI or GUI is implemented yet. Default path is the current directory.
Flags may appear before or after path; -- ends flag parsing.

Options:
  -h, --help                  Show this help; no config or filesystem scan
      --version               Show version, commit and optional build date
      --scan                  Explicit one-shot scan (also the R1 default)
      --json                  Emit scan metadata and warnings as JSON
      --ignore <pattern>      Repeatable gitignore-style rule; quote globs
      --ignore-file <path>    Additional file, relative to cwd; must exist
      --respect-gitignore     Enable root/nested .gitignore (default false)
      --include-git           Disable default .git/ exclusion (default false)
      --debounce <duration>   Positive Go duration (default 150ms; R2 reserved)
      --max-diff-bytes <size> Positive bytes/KB/MiB/etc (default 5MiB; R3 reserved)
      --no-mouse              Disable mouse (default false; R4 reserved)
      --editor <command>      Store editor text only; never execute in R1
      --log-file <path>       Validate destination only; never create in R1
      --debug                 Store debug setting only (R4 reserved)

Config: CLI flags > root .folderwatch.toml > user config > built-in defaults.
User config: OS user config directory/FolderWatch/config.toml
macOS: ~/Library/Application Support/FolderWatch/config.toml
A higher-priority ignore array replaces the lower array. Boolean flags accept
=false. Config-file paths resolve beside that file, CLI paths against cwd.
Root .folderwatchignore is always read; explicit ignore files are additive.
Rule priority: default .git/ < enabled .gitignore < .folderwatchignore < explicit
ignore file < config/CLI rules. Unignore an excluded parent before its children.
Descendant symlinks are reported but never followed. Rule edits require restart.
No state/config/log files or file contents are written by this R1 command.
Exit codes: 0 success (warnings possible), 2 input/config, 1 runtime/I/O, 130 cancel.
`
