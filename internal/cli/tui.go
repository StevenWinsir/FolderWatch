package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/logging"
	"github.com/StevenWinsir/FolderWatch/internal/tui"
	"github.com/charmbracelet/x/term"
)

func terminalAvailable(output io.Writer) bool {
	file, ok := output.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd()) && term.IsTerminal(os.Stdin.Fd())
}

func runTUI(ctx context.Context, prepared app.Prepared, stdout, stderr io.Writer) int {
	log, err := logging.Open(prepared.Config.LogFile, prepared.Config.Root, prepared.Config.Debug)
	if err != nil {
		diagnostic(stderr, err)
		return 1
	}
	defer log.Close()
	err = tui.Run(ctx, prepared, os.Stdin, stdout, log)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 130
	}
	if err != nil {
		diagnostic(stderr, err)
		return 1
	}
	return 0
}
