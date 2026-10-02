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
	Root             string
	Debounce         time.Duration
	Ignore           []string
	IgnoreFile       string
	RespectGitIgnore bool
	IncludeGit       bool
	NoMouse          bool
	MaxDiffBytes     int64
	Editor           string
	LogFile          string
	Debug            bool
}

// Overlay preserves presence, including explicit false and an empty ignore list.
type Overlay struct {
	Debounce         *string  `toml:"debounce"`
	Ignore           []string `toml:"ignore"`
	IgnoreFile       *string  `toml:"ignore_file"`
	RespectGitIgnore *bool    `toml:"respect_gitignore"`
	IncludeGit       *bool    `toml:"include_git"`
	NoMouse          *bool    `toml:"no_mouse"`
	MaxDiffBytes     *string  `toml:"max_diff_bytes"`
	Editor           *string  `toml:"editor"`
	LogFile          *string  `toml:"log_file"`
	Debug            *bool    `toml:"debug"`
}

// LoadOptions makes environment-dependent config discovery testable.
// UserConfigPath overrides discovery; a missing user config remains optional.
type LoadOptions struct {
	CWD            string
	UserConfigPath string
	SkipUserConfig bool
}

func Defaults() Config {
	return Config{Debounce: 150 * time.Millisecond, MaxDiffBytes: 5 << 20, Ignore: []string{}}
}

func (c Config) Validate() error {
	if c.Root == "" || !filepath.IsAbs(c.Root) || strings.ContainsRune(c.Root, 0) {
		return fmt.Errorf("root must be a normalized absolute directory")
	}
	if c.Debounce <= 0 {
		return fmt.Errorf("debounce must be positive")
	}
	if c.MaxDiffBytes <= 0 {
		return fmt.Errorf("max-diff-bytes must be positive")
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
