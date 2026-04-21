package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"docknudge/internal/config"
	"docknudge/internal/docker"
	"docknudge/internal/events"
	"docknudge/internal/incidents"
	"docknudge/internal/notifiers"
	"docknudge/internal/rules"
)

type Service struct {
	Config     config.Config
	ConfigPath string
	Source     docker.EventSource
	Dispatcher *notifiers.Dispatcher
	Logger     *slog.Logger
	Host       string

	engine    *rules.Engine
	incidents *incidents.Manager
	seen      *eventDeduper
}

func NewService(cfg config.Config, configPath string, source docker.EventSource, dispatcher *notifiers.Dispatcher, logger *slog.Logger, host string) *Service {
	return &Service{
		Config:     cfg,
		ConfigPath: configPath,
		Source:     source,
		Dispatcher: dispatcher,
		Logger:     logger,
		Host:       host,
		engine:     rules.New(cfg, host),
		incidents:  incidents.New(cfg.Cooldown.Duration),
		seen:       newEventDeduper(2048),
	}
}

func (s *Service) Run(ctx context.Context) error {
	defer s.Source.Close()

	if err := s.Source.Ping(ctx); err != nil {
		return err
	}

	s.Logger.Info("starting docknudge",
		"config_path", s.ConfigPath,
		"mode", "live_only",
		"docker_host", s.Config.DockerHost,
		"channels", s.Dispatcher.ChannelNames(),
		"rules", enabledRules(s.Config),
		"log_level", s.Config.LogLevel,
	)
	if s.Config.StartupBackfill.Duration > 0 {
		s.Logger.Warn("startup_backfill is configured but ignored in live-only mode",
			"startup_backfill", s.Config.StartupBackfill.String(),
		)
	}

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			s.Logger.Info("shutting down docknudge")
			return nil
		default:
		}

		streamCtx, cancel := context.WithCancel(ctx)
		stream, errs, err := s.Source.Stream(streamCtx)
		if err != nil {
			cancel()
			if err := waitWithBackoff(ctx, backoff); err != nil {
				return nil
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
		s.Logger.Info("docker event stream connected")

		streamErr := s.consumeStream(streamCtx, stream, errs)
		cancel()
		if streamErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}

		s.Logger.Warn("docker event stream disconnected", "error", streamErr, "backoff", backoff.String())
		if err := waitWithBackoff(ctx, backoff); err != nil {
			return nil
		}
		backoff = nextBackoff(backoff)
	}
}

func (s *Service) consumeStream(ctx context.Context, stream <-chan events.Event, errs <-chan error) error {
	for stream != nil || errs != nil {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			if err := s.handleEvent(ctx, event); err != nil {
				s.Logger.Error("dispatch alert", "error", err)
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("docker event stream ended")
}

func (s *Service) handleEvent(ctx context.Context, event events.Event) error {
	if s.seen.Seen(event.Key()) {
		return nil
	}
	for _, alert := range s.engine.Process(event) {
		if !s.incidents.Allow(alert) {
			continue
		}
		if err := s.Dispatcher.DispatchDefault(ctx, alert); err != nil {
			return err
		}
		s.incidents.Record(alert)
	}
	return nil
}

func waitWithBackoff(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextBackoff(current time.Duration) time.Duration {
	if current >= 30*time.Second {
		return 30 * time.Second
	}
	return current * 2
}

func enabledRules(cfg config.Config) []string {
	var rules []string
	if cfg.Rules.OOM.Enabled {
		rules = append(rules, "oom")
	}
	if cfg.Rules.Die.Enabled {
		rules = append(rules, "die")
	}
	if cfg.Rules.Unhealthy.Enabled {
		rules = append(rules, "unhealthy")
	}
	if cfg.Rules.RestartBurst.Enabled {
		rules = append(rules, "restart_burst")
	}
	if cfg.Rules.UnexpectedStop.Enabled {
		rules = append(rules, "unexpected_stop")
	}
	return rules
}

type eventDeduper struct {
	maxEntries int
	order      []string
	seen       map[string]struct{}
}

func newEventDeduper(maxEntries int) *eventDeduper {
	return &eventDeduper{
		maxEntries: maxEntries,
		seen:       map[string]struct{}{},
	}
}

func (d *eventDeduper) Seen(key string) bool {
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.order = append(d.order, key)
	d.seen[key] = struct{}{}
	if len(d.order) > d.maxEntries {
		evicted := d.order[0]
		d.order = d.order[1:]
		delete(d.seen, evicted)
	}
	return false
}
