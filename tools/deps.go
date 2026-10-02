//go:build tools

// Package tools pins the planned Terminal adapters without importing them into
// the R1 executable or core. R2/R3/R4 will move these imports to real adapters.
package tools

import (
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/lipgloss"
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/pmezard/go-difflib/difflib"
)
