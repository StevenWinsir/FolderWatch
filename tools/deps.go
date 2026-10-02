//go:build tools

// Package tools pins the planned Terminal adapters without importing them into
// the core. R2 uses fsnotify in its adapter; R3/R4 will move the remaining pins.
package tools

import (
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/lipgloss"
	_ "github.com/pmezard/go-difflib/difflib"
)
