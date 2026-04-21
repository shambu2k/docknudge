package cli_test

import (
	"bytes"
	"context"
	"testing"

	"docknudge/internal/cli"
)

func TestAppInitCommand(t *testing.T) {
	exec := &fakeExecutor{}
	var stdout, stderr bytes.Buffer
	app := cli.NewApp(exec, &stdout, &stderr)

	if code := app.Run(context.Background(), []string{"init"}); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr.String())
	}
	if !exec.initCalled {
		t.Fatal("expected init to be called")
	}
}

func TestAppValidateCommand(t *testing.T) {
	exec := &fakeExecutor{}
	var stdout, stderr bytes.Buffer
	app := cli.NewApp(exec, &stdout, &stderr)

	if code := app.Run(context.Background(), []string{"validate", "-c", "custom.yml"}); code != 0 {
		t.Fatalf("validate exit code = %d, stderr = %s", code, stderr.String())
	}
	if exec.validatePath != "custom.yml" {
		t.Fatalf("validate path = %q", exec.validatePath)
	}
}

func TestAppTestCommand(t *testing.T) {
	exec := &fakeExecutor{}
	var stdout, stderr bytes.Buffer
	app := cli.NewApp(exec, &stdout, &stderr)

	if code := app.Run(context.Background(), []string{"test", "-c", "custom.yml", "slack_ops"}); code != 0 {
		t.Fatalf("test exit code = %d, stderr = %s", code, stderr.String())
	}
	if exec.testPath != "custom.yml" || exec.testChannel != "slack_ops" {
		t.Fatalf("unexpected test call: path=%q channel=%q", exec.testPath, exec.testChannel)
	}
}

func TestAppVersionCommand(t *testing.T) {
	exec := &fakeExecutor{version: "docknudge 1.2.3"}
	var stdout, stderr bytes.Buffer
	app := cli.NewApp(exec, &stdout, &stderr)

	if code := app.Run(context.Background(), []string{"version"}); code != 0 {
		t.Fatalf("version exit code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.String() != "docknudge 1.2.3\n" {
		t.Fatalf("unexpected version output: %q", stdout.String())
	}
}

type fakeExecutor struct {
	initCalled   bool
	validatePath string
	testPath     string
	testChannel  string
	version      string
}

func (f *fakeExecutor) Init(string, bool) error {
	f.initCalled = true
	return nil
}

func (f *fakeExecutor) Validate(_ context.Context, path string) error {
	f.validatePath = path
	return nil
}

func (f *fakeExecutor) Test(_ context.Context, path, channel string) error {
	f.testPath = path
	f.testChannel = channel
	return nil
}

func (f *fakeExecutor) Run(context.Context, string) error {
	return nil
}

func (f *fakeExecutor) Version() string {
	if f.version != "" {
		return f.version
	}
	return "docknudge dev"
}
