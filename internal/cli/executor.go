package cli

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"docknudge/internal/config"
	"docknudge/internal/docker"
	"docknudge/internal/logging"
	"docknudge/internal/notifiers"
	"docknudge/internal/runtime"
)

//go:embed templates/docknudge.yml
var starterConfig string

type DefaultExecutor struct {
	version   string
	commit    string
	buildTime string
}

func NewDefaultExecutor(version, commit, buildTime string) *DefaultExecutor {
	return &DefaultExecutor{
		version:   version,
		commit:    commit,
		buildTime: buildTime,
	}
}

func (e *DefaultExecutor) Init(path string, force bool) error {
	target := config.ResolvePath(path)
	if !force {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%s already exists; use --force to overwrite", target)
		}
	}
	return os.WriteFile(target, []byte(starterConfig), 0o644)
}

func (e *DefaultExecutor) Validate(ctx context.Context, path string) error {
	cfg, _, err := e.loadConfig(path)
	if err != nil {
		return err
	}
	source, err := docker.New(cfg.DockerHost)
	if err != nil {
		return err
	}
	defer source.Close()
	return source.Ping(ctx)
}

func (e *DefaultExecutor) Test(ctx context.Context, path, channel string) error {
	cfg, logger, err := e.loadConfig(path)
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("resolve hostname: %w", err)
	}
	dispatcher, err := notifiers.NewDispatcher(cfg, logger, notifiers.NewHTTPClient(5*time.Second))
	if err != nil {
		return err
	}
	alert := notifiers.Alert{
		RuleName:         "test",
		Severity:         "info",
		Host:             hostname,
		ContainerID:      "docknudge",
		ContainerName:    "docknudge",
		ContainerIDShort: "docknudge",
		EventType:        "test",
		OccurredAt:       time.Now().UTC(),
		Summary:          fmt.Sprintf("[info] %s / docknudge - test alert from DockNudge", hostname),
	}
	return dispatcher.SendChannel(ctx, channel, alert)
}

func (e *DefaultExecutor) Run(ctx context.Context, path string) error {
	cfg, logger, err := e.loadConfig(path)
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("resolve hostname: %w", err)
	}
	source, err := docker.New(cfg.DockerHost)
	if err != nil {
		return err
	}
	dispatcher, err := notifiers.NewDispatcher(cfg, logger, notifiers.NewHTTPClient(5*time.Second))
	if err != nil {
		_ = source.Close()
		return err
	}

	service := runtime.NewService(cfg, config.ResolvePath(path), source, dispatcher, logger, hostname)

	runCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return service.Run(runCtx)
}

func (e *DefaultExecutor) Version() string {
	return strings.TrimSpace(fmt.Sprintf(`
docknudge %s
commit: %s
built: %s
`, e.version, e.commit, e.buildTime))
}

func (e *DefaultExecutor) loadConfig(path string) (config.Config, *slog.Logger, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, logger, nil
}
