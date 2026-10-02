// Package ignore implements shared initial-scan/runtime gitignore-style rules.
package ignore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/StevenWinsir/FolderWatch/internal/fileutil"
	"github.com/StevenWinsir/FolderWatch/internal/pathutil"
)

// Filter is consumed by both the initial scanner and future runtime adapters.
// Match accepts absolute or root-relative paths and knows the final entry kind.
type Filter interface {
	Match(path string, isDir bool) (bool, error)
}

type Options struct {
	Root             string
	Patterns         []string
	IgnoreFile       string
	RespectGitIgnore bool
	IncludeGit       bool
}

type ruleSet struct {
	identity os.FileInfo
	rules    []rule
	err      error
}

// Matcher is concurrent-safe. Rule files (including missing files) are cached
// per session; create a new matcher to reload existing ignore files.
type Matcher struct {
	root       string
	respectGit bool
	builtin    []rule
	custom     []rule
	mu         sync.Mutex
	git        map[string]ruleSet
}

func New(opts Options) (*Matcher, error) {
	root, err := pathutil.NormalizeRoot(opts.Root, ".")
	if err != nil {
		return nil, err
	}
	m := &Matcher{root: root, respectGit: opts.RespectGitIgnore, git: make(map[string]ruleSet)}
	if !opts.IncludeGit {
		m.builtin, _ = parseRules([]string{".git/"}, ".", "built-in")
	}
	if m.respectGit {
		if _, err := m.gitRules("."); err != nil {
			return nil, err
		}
	}
	rules, err := readRules(filepath.Join(root, ".folderwatchignore"), ".", true, false)
	if err != nil {
		return nil, err
	}
	m.custom = append(m.custom, rules...)
	if opts.IgnoreFile != "" {
		rules, err = readRules(opts.IgnoreFile, ".", false, true)
		if err != nil {
			return nil, err
		}
		m.custom = append(m.custom, rules...)
	}
	rules, err = parseRules(opts.Patterns, ".", "config/CLI ignore")
	if err != nil {
		return nil, err
	}
	m.custom = append(m.custom, rules...)
	return m, nil
}

func readRules(file, base string, optional, allowSymlink bool) ([]rule, error) {
	data, err := fileutil.ReadConfig(file, allowSymlink)
	if optional && os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file %q: %w", file, err)
	}
	return parseRules(strings.Split(string(data), "\n"), base, file)
}

func (m *Matcher) gitRules(base string) ([]rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dir := filepath.Join(m.root, filepath.FromSlash(base))
	identity, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		delete(m.git, base) // missing directories are not session rule scopes
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !identity.IsDir() {
		return nil, fmt.Errorf("ignore scope %q is not a directory", dir)
	}
	if set, ok := m.git[base]; ok && os.SameFile(identity, set.identity) {
		return set.rules, set.err
	}
	rules, err := readRules(filepath.Join(dir, ".gitignore"), base, true, false)
	m.git[base] = ruleSet{identity: identity, rules: rules, err: err}
	return rules, err
}

// ForgetDirectory retires a removed/replaced directory scope, not edits within
// an existing scope. The watcher calls it when directory identity disappears.
func (m *Matcher) ForgetDirectory(input string) {
	key, err := pathutil.Key(m.root, input)
	if err != nil || key == "." {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for base := range m.git {
		if base == key || strings.HasPrefix(base, key+"/") {
			delete(m.git, base)
		}
	}
}

// Match evaluates ancestors first. An excluded parent cannot be resurrected by
// a child-only negation. New directories load nested Git rules on first use.
func (m *Matcher) Match(input string, isDir bool) (bool, error) {
	key, err := pathutil.Key(m.root, input)
	if err != nil {
		return false, err
	}
	parentCheck := key
	// A directory hint does not authorize reading through a final symlink.
	if isDir {
		parentCheck += "/_"
	}
	if err := pathutil.CheckParents(m.root, parentCheck); err != nil {
		return false, err
	}
	if key == "." {
		return false, nil
	}
	parts := strings.Split(key, "/")
	var gitRules []rule
	for i := range parts {
		if m.respectGit {
			base := "."
			if i > 0 {
				base = strings.Join(parts[:i], "/")
			}
			rules, err := m.gitRules(base)
			if err != nil {
				return false, err
			}
			gitRules = append(gitRules, rules...)
		}
		node := strings.Join(parts[:i+1], "/")
		directory := i < len(parts)-1 || isDir
		ignored := evaluate(m.builtin, node, directory, false)
		ignored = evaluate(gitRules, node, directory, ignored)
		ignored = evaluate(m.custom, node, directory, ignored)
		if ignored {
			return true, nil
		}
	}
	// Validate a kept directory's own rules before the scanner enters it.
	// Those rules affect children only; they cannot ignore their own scope.
	if isDir && m.respectGit {
		if _, err := m.gitRules(key); err != nil {
			return false, err
		}
	}
	return false, nil
}
