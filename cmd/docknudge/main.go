package main

import (
	"context"
	"os"

	"docknudge/internal/cli"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	exec := cli.NewDefaultExecutor(version, commit, buildTime)
	app := cli.NewApp(exec, os.Stdout, os.Stderr)
	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
