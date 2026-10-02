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
	Watch   bool
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
	fs.BoolVar(&r.Watch, "watch", false, "Monitor with settled path events and a baseline")
	var pending, dirs, files int
	var memory, cache string
	fs.IntVar(&pending, "max-pending-events", 4096, "Bounded pending event capacity")
	fs.IntVar(&dirs, "max-watch-dirs", 8192, "Maximum watched directories")
	fs.IntVar(&files, "max-snapshot-files", 100000, "Maximum snapshot references")
	fs.StringVar(&memory, "snapshot-memory-bytes", "32MiB", "Total retained memory budget per generation")
	fs.StringVar(&cache, "snapshot-cache-bytes", "256MiB", "Total retained disk budget per generation")
	fs.StringVar(&debounce, "debounce", "150ms", "Positive event-settling duration")
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
	if r.Watch && r.Scan {
		return Request{}, fmt.Errorf("--watch and --scan cannot be combined")
	}
	if fs.Changed("max-pending-events") {
		r.Overlay.MaxPendingEvents = &pending
	}
	if fs.Changed("max-watch-dirs") {
		r.Overlay.MaxWatchDirs = &dirs
	}
	if fs.Changed("max-snapshot-files") {
		r.Overlay.MaxSnapshotFiles = &files
	}
	if fs.Changed("snapshot-memory-bytes") {
		r.Overlay.SnapshotMemoryBytes = &memory
	}
	if fs.Changed("snapshot-cache-bytes") {
		r.Overlay.SnapshotCacheBytes = &cache
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

R2 (P0–P5): default/--scan scans once; --watch captures a baseline and emits
settled path invalidations until Ctrl+C. --watch --json emits NDJSON, not a
semantic changed-file list. Text diff, TUI and GUI are not implemented yet.
Default path is the current directory. ResetBaseline is available in the Go API.
Flags may appear before or after path; -- ends flag parsing.

Options:
  -h, --help                  Show this help; no config or filesystem scan
      --version               Show version, commit and optional build date
      --scan                  Explicit one-shot scan (also the default)
      --watch                 Capture baseline and continuously report invalidations
      --json                  Scan JSON or streaming --watch NDJSON
      --ignore <pattern>      Repeatable gitignore-style rule; quote globs
      --ignore-file <path>    Additional file, relative to cwd; must exist
      --respect-gitignore     Enable root/nested .gitignore (default false)
      --include-git           Disable default .git/ exclusion (default false)
      --debounce <duration>   Positive settling duration (default 150ms; maximum wait 4x)
      --max-diff-bytes <size> Per-file retained snapshot cap (default 5MiB); future diff cap
      --no-mouse              Disable mouse (default false; R4 reserved)
      --editor <command>      Store editor text only; not executed
      --log-file <path>       Validate destination only; not created
      --debug                 Store debug setting only (R4 reserved)

Resource limits (also snake_case TOML keys):
      --max-pending-events <n>       Default 4096; overflow requires reconciliation
      --max-watch-dirs <n>           Default 8192
      --max-snapshot-files <n>       Default 100000
      --snapshot-memory-bytes <size> Default 32MiB per generation
      --snapshot-cache-bytes <size>  Default 256MiB per generation
Reset can temporarily retain two bounded generations. Files over limits still
receive metadata/hash; binary bytes are not retained. Watch startup/reset needs
a complete readable baseline; --scan continues with partial-scan warnings.

Config: CLI flags > root .folderwatch.toml > user config > built-in defaults.
User config: OS user config directory/FolderWatch/config.toml
macOS: ~/Library/Application Support/FolderWatch/config.toml
A higher-priority ignore array replaces the lower array. Boolean flags accept
=false. Config-file paths resolve beside that file, CLI paths against cwd.
Root .folderwatchignore is always read; explicit ignore files are additive.
Rule priority: default .git/ < enabled .gitignore < .folderwatchignore < explicit
ignore file < config/CLI rules. Unignore an excluded parent before its children.
Descendant symlinks are reported but never followed. Rule edits require restart.
No files are written under the monitored root. --watch uses a private OS temp
cache outside root, removed at Close. Default/--scan never reads file contents.
Exit codes: 0 success (warnings possible), 2 input/config, 1 runtime/I/O, 130 cancel.
`
