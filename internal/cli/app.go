package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

type Executor interface {
	Init(path string, force bool) error
	Validate(ctx context.Context, path string) error
	Test(ctx context.Context, path, channel string) error
	Run(ctx context.Context, path string) error
	Version() string
}

type App struct {
	exec   Executor
	stdout io.Writer
	stderr io.Writer
}

func NewApp(exec Executor, stdout, stderr io.Writer) App {
	return App{exec: exec, stdout: stdout, stderr: stderr}
}

func (a App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.printUsage()
		return 1
	}

	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		fs.SetOutput(a.stderr)
		force := fs.Bool("force", false, "overwrite an existing docknudge.yml")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := a.exec.Init("", *force); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		fmt.Fprintln(a.stdout, "wrote docknudge.yml")
		return 0
	case "validate":
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		fs.SetOutput(a.stderr)
		configPath := fs.String("c", "", "path to config file")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := a.exec.Validate(ctx, *configPath); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		fmt.Fprintln(a.stdout, "config is valid")
		return 0
	case "test":
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(a.stderr)
		configPath := fs.String("c", "", "path to config file")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if fs.NArg() != 1 {
			fmt.Fprintln(a.stderr, "usage: docknudge test [-c path] <channel-name>")
			return 1
		}
		channel := fs.Arg(0)
		if err := a.exec.Test(ctx, *configPath, channel); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		fmt.Fprintf(a.stdout, "test alert sent to %s\n", channel)
		return 0
	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		fs.SetOutput(a.stderr)
		configPath := fs.String("c", "", "path to config file")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := a.exec.Run(ctx, *configPath); err != nil {
			fmt.Fprintln(a.stderr, err)
			return 1
		}
		return 0
	case "version":
		fmt.Fprintln(a.stdout, a.exec.Version())
		return 0
	case "help", "--help", "-h":
		a.printUsage()
		return 0
	default:
		fmt.Fprintf(a.stderr, "unknown command %q\n", args[0])
		a.printUsage()
		return 1
	}
}

func (a App) printUsage() {
	fmt.Fprintln(a.stderr, strings.TrimSpace(`
DockNudge watches Docker container events and sends Slack or Google Chat alerts.

Usage:
  docknudge init [--force]
  docknudge validate [-c path]
  docknudge test [-c path] <channel-name>
  docknudge run [-c path]
  docknudge version
`))
}
