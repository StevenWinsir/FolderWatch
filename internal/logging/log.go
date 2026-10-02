// Package logging provides bounded, content-free diagnostics, never terminal output.
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	Capacity        = 200
	MaxMessageBytes = 2048
	MaxFileBytes    = 4 << 20
)

type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type Logger struct {
	mu      sync.Mutex
	debug   bool
	entries []Entry
	file    *os.File
	written int
	err     error
}

// Open never chooses a default file. Explicit destinations must be NEW files
// outside the watched root, preventing feedback loops, symlink writes and data
// truncation. The resolved parent is used for creation; no directories are made.
func Open(path, root string, debug bool) (*Logger, error) {
	l := &Logger{debug: debug}
	if path == "" {
		return l, nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("log directory: %w", err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(parent, filepath.Base(path))
	rel, err := filepath.Rel(root, destination)
	if err != nil {
		return nil, err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("log file must be outside the monitored root")
	}
	// A case-insensitive filesystem may resolve a differently cased root spelling
	// to the same directory without changing its lexical path. Compare identities.
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Stat(ancestor)
		if err != nil {
			return nil, err
		}
		if os.SameFile(rootInfo, info) {
			return nil, fmt.Errorf("log file must be outside the monitored root")
		}
		if next := filepath.Dir(ancestor); next == ancestor {
			break
		}
	}
	l.file, err = os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create new private log (existing files are not overwritten): %w", err)
	}
	return l, nil
}

// Record accepts metadata/diagnostics only. Callers must never pass diff or content.
func (l *Logger) Record(level, message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if level == "debug" && !l.debug {
		return
	}
	if len(message) > MaxMessageBytes {
		message = strings.ToValidUTF8(message[:MaxMessageBytes], "") + " [truncated]"
	}
	e := Entry{Time: time.Now().UTC(), Level: level, Message: message}
	l.append(e)
	if l.file == nil || l.err != nil {
		return
	}
	b, err := json.Marshal(e)
	if err == nil && l.written+len(b)+1 > MaxFileBytes {
		err = fmt.Errorf("log reached 4MiB limit; file logging stopped, diagnostics ring remains available")
	}
	if err == nil {
		var n int
		n, err = l.file.Write(append(b, '\n'))
		l.written += n
	}
	if err != nil {
		l.err = err
		l.append(Entry{Time: time.Now().UTC(), Level: "warning", Message: err.Error()})
	}
}
func (l *Logger) append(e Entry) {
	if len(l.entries) == Capacity {
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = e
	} else {
		l.entries = append(l.entries, e)
	}
}
func (l *Logger) Entries() []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry(nil), l.entries...)
}
func (l *Logger) Err() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
