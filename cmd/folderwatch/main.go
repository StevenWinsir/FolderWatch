package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/StevenWinsir/FolderWatch/internal/cli"
	"github.com/StevenWinsir/FolderWatch/internal/config"
)

var version = "dev"
var commit = "unknown"
var buildDate = ""

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr,
		cli.BuildInfo{Version: version, Commit: commit, Date: buildDate}, config.LoadOptions{})
	stop()
	os.Exit(code)
}
