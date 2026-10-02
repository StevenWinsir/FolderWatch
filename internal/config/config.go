// Package config is the shared configuration contract for all UI adapters.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/ignore"
)

const ProjectFile = ".folderwatch.toml"

// Config contains normalized values, independent of any CLI/UI library.
type Config struct {
	Root                string
	Debounce            time.Duration
	Ignore              []string
	IgnoreFile          string
	RespectGitIgnore    bool
	IncludeGit          bool
	NoMouse             bool
	MaxDiffBytes        int64
	MaxSnapshotBytes    int64
	MaxDiffLines        int
	MaxPendingEvents    int
	MaxWatchDirs        int
	MaxSnapshotFiles    int
	SnapshotMemoryBytes int64
	SnapshotCacheBytes  int64
	Editor              string
	LogFile             string
	Debug               bool
}

// Overlay preserves presence, including explicit false and an empty ignore list.
type Overlay struct {
	Debounce            *string  `toml:"debounce"`
	Ignore              []string `toml:"ignore"`
	IgnoreFile          *string  `toml:"ignore_file"`
	RespectGitIgnore    *bool    `toml:"respect_gitignore"`
	IncludeGit          *bool    `toml:"include_git"`
	NoMouse             *bool    `toml:"no_mouse"`
	MaxDiffBytes        *string  `toml:"max_diff_bytes"`
	MaxSnapshotBytes    *string  `toml:"max_snapshot_bytes"`
	MaxDiffLines        *int     `toml:"max_diff_lines"`
	MaxPendingEvents    *int     `toml:"max_pending_events"`
	MaxWatchDirs        *int     `toml:"max_watch_dirs"`
	MaxSnapshotFiles    *int     `toml:"max_snapshot_files"`
	SnapshotMemoryBytes *string  `toml:"snapshot_memory_bytes"`
	SnapshotCacheBytes  *string  `toml:"snapshot_cache_bytes"`
	Editor              *string  `toml:"editor"`
	LogFile             *string  `toml:"log_file"`
	Debug               *bool    `toml:"debug"`
}

// LoadOptions makes environment-dependent config discovery testable.
// UserConfigPath overrides discovery; a missing user config remains optional.
type LoadOptions struct {
	CWD            string
	UserConfigPath string
	SkipUserConfig bool
}

func Defaults() Config {
	return Config{Debounce: 150 * time.Millisecond, MaxDiffBytes: 5 << 20, MaxSnapshotBytes: 8 << 20, MaxDiffLines: 20000, Ignore: []string{}, MaxPendingEvents: 4096, MaxWatchDirs: 8192, MaxSnapshotFiles: 100000, SnapshotMemoryBytes: 32 << 20, SnapshotCacheBytes: 256 << 20}
}

func (c Config) Validate() error {
	if c.Root == "" || !filepath.IsAbs(c.Root) || strings.ContainsRune(c.Root, 0) {
		return fmt.Errorf("root must be a normalized absolute directory")
	}
	if c.Debounce <= 0 || c.Debounce > time.Duration(1<<63-1)/4 {
		return fmt.Errorf("debounce must be positive and allow a 4x settling window without overflow")
	}
	if c.MaxPendingEvents < 1 || c.MaxPendingEvents > 1<<20 || c.MaxWatchDirs < 1 || c.MaxSnapshotFiles < 1 {
		return fmt.Errorf("pending events must be 1..1048576; directory and snapshot entry limits must be positive")
	}
	if c.SnapshotMemoryBytes <= 0 || c.SnapshotCacheBytes <= 0 {
		return fmt.Errorf("snapshot memory/cache budgets must be positive")
	}
	if c.MaxDiffBytes <= 0 || c.MaxDiffBytes > 64<<20 {
		return fmt.Errorf("max-diff-bytes must be 1..64MiB")
	}
	if c.MaxSnapshotBytes <= 0 || c.MaxSnapshotBytes > 64<<20 {
		return fmt.Errorf("max-snapshot-bytes must be 1..64MiB")
	}
	if c.MaxDiffLines < 1 || c.MaxDiffLines > 200000 {
		return fmt.Errorf("max-diff-lines must be 1..200000")
	}
	for _, pattern := range c.Ignore {
		if strings.TrimSpace(pattern) == "" || strings.ContainsAny(pattern, "\x00\r\n") {
			return fmt.Errorf("ignore pattern %q must be a non-empty single line", pattern)
		}
	}
	if strings.ContainsRune(c.Editor, 0) {
		return fmt.Errorf("editor must not contain NUL bytes")
	}
	return ignore.ValidatePatterns(c.Ignore)
}
