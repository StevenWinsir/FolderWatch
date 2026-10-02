package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/StevenWinsir/FolderWatch/internal/app"
	"github.com/StevenWinsir/FolderWatch/internal/config"
	"github.com/StevenWinsir/FolderWatch/internal/scan"
)

type BuildInfo struct{ Version, Commit, Date string }

// Run returns a process exit code without terminating the caller.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, build BuildInfo, opts config.LoadOptions) int {
	request, err := Parse(args)
	if err != nil {
		diagnostic(stderr, err)
		return 2
	}
	if request.Help {
		if _, err := io.WriteString(stdout, HelpText); err != nil {
			diagnostic(stderr, err)
			return 1
		}
		return 0
	}
	if request.Version {
		_, err := fmt.Fprintf(stdout, "folderwatch %s (commit %s, built %s)\n", build.Version, build.Commit, build.Date)
		if err != nil {
			diagnostic(stderr, err)
			return 1
		}
		return 0
	}
	prepared, err := app.Prepare(ctx, request.Root, request.Overlay, opts)
	if err != nil {
		diagnostic(stderr, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		var input *app.InputError
		if errors.As(err, &input) {
			return 2
		}
		return 1
	}
	if request.JSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(prepared.Inventory)
	} else {
		err = renderText(stdout, prepared.Inventory)
	}
	if err != nil {
		diagnostic(stderr, err)
		return 1
	}
	for _, warning := range prepared.Inventory.Warnings {
		if _, err := fmt.Fprintf(stderr, "warning: %q: %q; scanning continues for other entries\n", warning.Path, warning.Message); err != nil {
			return 1
		}
	}
	return 0
}

func diagnostic(w io.Writer, err error) {
	// Quote all diagnostics: filesystem names can contain terminal controls.
	_, _ = fmt.Fprintf(w, "folderwatch: %q\n", err.Error())
}

func renderText(w io.Writer, result scan.Result) error {
	buffer := bufio.NewWriter(w)
	fmt.Fprintln(buffer, "FolderWatch — initial scan only (R1; no live watcher/baseline)")
	fmt.Fprintf(buffer, "Root: %q\n", result.Root)
	for _, entry := range result.Entries {
		fmt.Fprintf(buffer, "%-9s %q\n", entry.Kind, entry.Path)
	}
	fmt.Fprintf(buffer, "%d entries (including root), %d warnings\n", len(result.Entries), len(result.Warnings))
	return buffer.Flush()
}
