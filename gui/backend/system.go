package backend

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

// systemActions is kept behind function values so all path and command
// validation can be tested without launching an editor or touching a desktop.
type systemActions struct {
	openEditor func(context.Context, string, string) error
	reveal     func(context.Context, string) error
	copy       func(context.Context, string) error
}

func defaultSystemActions() systemActions {
	return systemActions{openEditor: launchEditor, reveal: revealInFinder, copy: copyToClipboard}
}

func commandParts(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n;&|<>`") {
		return nil, fmt.Errorf("editor command is empty or contains a prohibited shell character")
	}
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			started = true
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			started = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				parts = append(parts, current.String())
				current.Reset()
				started = false
			}
			continue
		}
		current.WriteRune(r)
		started = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("editor command has an unterminated quote or escape")
	}
	if started {
		parts = append(parts, current.String())
	}
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("editor command is empty")
	}
	return parts, nil
}

func startCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func launchEditor(ctx context.Context, editor, absolute string) error {
	if editor == "" {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("default editor integration is only available on macOS; configure an editor command")
		}
		return startCommand(ctx, "open", absolute)
	}
	parts, err := commandParts(editor)
	if err != nil {
		return err
	}
	return startCommand(ctx, parts[0], append(parts[1:], absolute)...)
}

func revealInFinder(ctx context.Context, absolute string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("Reveal in Finder is only available on macOS")
	}
	return startCommand(ctx, "open", "-R", absolute)
}

func copyToClipboard(ctx context.Context, value string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("clipboard integration is only available on macOS")
	}
	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}
