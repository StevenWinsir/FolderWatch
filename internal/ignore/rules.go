package ignore

import (
	"fmt"
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type rule struct {
	base          string
	pattern       string
	negated       bool
	directoryOnly bool
	anchored      bool
}

// ValidatePatterns checks the same grammar used by Match without reading disk.
func ValidatePatterns(patterns []string) error {
	_, err := parseRules(patterns, ".", "ignore")
	return err
}

func parseRules(lines []string, base, source string) ([]rule, error) {
	var rules []rule
	for i, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		if i == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		line = trimTrailingSpaces(line)
		if strings.ContainsRune(line, 0) {
			return nil, fmt.Errorf("%s:%d: NUL in ignore rule", source, i+1)
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := rule{base: base}
		if strings.HasPrefix(line, "!") {
			r.negated = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.directoryOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		r.anchored = strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		r.pattern = literalBraces(line)
		// Git's trailing /** means descendants, not the directory itself.
		// doublestar otherwise allows its final ** to consume zero segments.
		if strings.HasSuffix(r.pattern, "/**") {
			r.pattern += "/*"
		}
		if !doublestar.ValidatePattern(r.pattern) {
			return nil, fmt.Errorf("%s:%d: invalid ignore glob %q", source, i+1, raw)
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func trimTrailingSpaces(s string) string {
	for strings.HasSuffix(s, " ") {
		backslashes := 0
		for i := len(s) - 2; i >= 0 && s[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// doublestar offers brace alternatives; gitignore treats braces literally.
func literalBraces(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i])
			i++
			b.WriteByte(s[i])
			continue
		}
		if s[i] == '{' || s[i] == '}' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (r rule) matches(key string, isDir bool) bool {
	if r.directoryOnly && !isDir {
		return false
	}
	target := key
	if r.base != "." {
		prefix := r.base + "/"
		if !strings.HasPrefix(key, prefix) {
			return false
		}
		target = strings.TrimPrefix(key, prefix)
	}
	if !r.anchored {
		target = path.Base(target)
	}
	matched, _ := doublestar.Match(r.pattern, target)
	return matched
}

func evaluate(rules []rule, key string, isDir, ignored bool) bool {
	for _, r := range rules {
		if r.matches(key, isDir) {
			ignored = !r.negated
		}
	}
	return ignored
}
