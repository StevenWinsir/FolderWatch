package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/StevenWinsir/FolderWatch/internal/fileutil"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

func apply(cfg *Config, o Overlay, base string) error {
	if o.MaxPendingEvents != nil {
		cfg.MaxPendingEvents = *o.MaxPendingEvents
	}
	if o.MaxWatchDirs != nil {
		cfg.MaxWatchDirs = *o.MaxWatchDirs
	}
	if o.MaxSnapshotFiles != nil {
		cfg.MaxSnapshotFiles = *o.MaxSnapshotFiles
	}
	if o.SnapshotMemoryBytes != nil {
		n, err := ParseSize(*o.SnapshotMemoryBytes)
		if err != nil {
			return fmt.Errorf("snapshot-memory-bytes: %w", err)
		}
		cfg.SnapshotMemoryBytes = n
	}
	if o.SnapshotCacheBytes != nil {
		n, err := ParseSize(*o.SnapshotCacheBytes)
		if err != nil {
			return fmt.Errorf("snapshot-cache-bytes: %w", err)
		}
		cfg.SnapshotCacheBytes = n
	}
	if o.Debounce != nil {
		d, err := time.ParseDuration(*o.Debounce)
		if err != nil || d <= 0 {
			return fmt.Errorf("debounce %q must be a positive duration, e.g. 150ms", *o.Debounce)
		}
		cfg.Debounce = d
	}
	if o.MaxDiffBytes != nil {
		n, err := ParseSize(*o.MaxDiffBytes)
		if err != nil {
			return fmt.Errorf("max-diff-bytes: %w", err)
		}
		cfg.MaxDiffBytes = n
	}
	if o.Ignore != nil {
		cfg.Ignore = append([]string{}, o.Ignore...)
	}
	if o.IgnoreFile != nil {
		cfg.IgnoreFile = ""
		if *o.IgnoreFile != "" {
			p, err := pathutil.Resolve(*o.IgnoreFile, base)
			if err != nil {
				return fmt.Errorf("ignore-file: %w", err)
			}
			if _, err := fileutil.ReadConfig(p, true); err != nil {
				return fmt.Errorf("ignore-file %q: %w", p, err)
			}
			cfg.IgnoreFile = p
		}
	}
	if o.LogFile != nil {
		cfg.LogFile = ""
		if *o.LogFile != "" {
			p, err := logPath(*o.LogFile, base)
			if err != nil {
				return err
			}
			cfg.LogFile = p
		}
	}
	if o.RespectGitIgnore != nil {
		cfg.RespectGitIgnore = *o.RespectGitIgnore
	}
	if o.IncludeGit != nil {
		cfg.IncludeGit = *o.IncludeGit
	}
	if o.NoMouse != nil {
		cfg.NoMouse = *o.NoMouse
	}
	if o.Debug != nil {
		cfg.Debug = *o.Debug
	}
	if o.Editor != nil {
		cfg.Editor = *o.Editor
	}
	return cfg.Validate()
}

func logPath(value, base string) (string, error) {
	p, err := pathutil.Resolve(value, base)
	if err != nil {
		return "", fmt.Errorf("log-file: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(p))
	if err != nil || !parent.IsDir() {
		return "", fmt.Errorf("log-file %q: parent must be an existing directory", p)
	}
	info, err := os.Stat(p)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("log-file %q: %w", p, err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("log-file %q must be a regular file destination", p)
	}
	return p, nil
}
