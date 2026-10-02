// Package pathutil defines FolderWatch's canonical root-relative path policy.
package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve expands the current user's home and resolves relative input at base.
// It does not require the final path to exist or expand environment variables.
func Resolve(input, base string) (string, error) {
	if input == "" || strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("path must be non-empty and contain no NUL bytes")
	}
	if strings.HasPrefix(input, "~") {
		if input != "~" && !strings.HasPrefix(input, "~/") && !strings.HasPrefix(input, "~"+string(filepath.Separator)) {
			return "", fmt.Errorf("path %q: ~otheruser expansion is not supported", input)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if input == "~" {
			input = home
		} else {
			input = filepath.Join(home, input[2:])
		}
	}
	if !filepath.IsAbs(input) {
		input = filepath.Join(base, input)
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", input, err)
	}
	return filepath.Clean(absolute), nil
}

// NormalizeRoot accepts an explicitly selected directory symlink, resolving it
// once to a real absolute root. Descendant symlinks are not followed by scans.
func NormalizeRoot(input, base string) (string, error) {
	root, err := Resolve(input, base)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", input, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", input, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root %q is not a directory", input)
	}
	return filepath.Clean(root), nil
}

// Key returns a slash-separated relative key (root itself is "."). Input may
// be absolute or root-relative. It is lexical: use CheckParents before reads.
func Key(root, input string) (string, error) {
	if !filepath.IsAbs(root) || input == "" || strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("invalid root or path %q", input)
	}
	if !filepath.IsAbs(input) {
		input = filepath.Join(root, filepath.FromSlash(input))
	}
	rel, err := filepath.Rel(root, filepath.Clean(input))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q is outside root %q", input, root)
	}
	return filepath.ToSlash(rel), nil
}

// CheckParents rejects existing symlink ancestors, including a replaced root.
// Missing ancestors are allowed for future-create/deleted-path filtering. The
// final component may be a symlink: callers must Lstat it, never read through it.
// This check cannot make subsequent filesystem operations atomic against races.
func CheckParents(root, key string) error {
	rel, err := Key(root, key)
	if err != nil {
		return err
	}
	parts := strings.Split(rel, "/")
	current := root
	for i := 0; i < len(parts); i++ {
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink ancestor %q is not followed", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("ancestor %q is not a directory", current)
		}
		current = filepath.Join(current, parts[i])
	}
	return nil
}
