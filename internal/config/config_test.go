package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"docknudge/internal/config"
)

func TestLoadAppliesDefaultsAndExpandsEnv(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/services/abc")

	path := writeConfig(t, `
version: 1
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [slack_ops]
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.StartupBackfill.Duration != 0 {
		t.Fatalf("StartupBackfill = %s", cfg.StartupBackfill)
	}
	if cfg.Cooldown.Duration != 10*time.Minute {
		t.Fatalf("Cooldown = %s", cfg.Cooldown)
	}
	if cfg.Channels["slack_ops"].WebhookURL != os.Getenv("SLACK_WEBHOOK_URL") {
		t.Fatalf("webhook URL was not expanded")
	}
	if !cfg.Rules.OOM.Enabled || !cfg.Rules.Unhealthy.Enabled || !cfg.Rules.UnexpectedStop.Enabled {
		t.Fatalf("default rules should be enabled: %+v", cfg.Rules)
	}
}

func TestLoadRejectsDuplicateKeys(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/services/abc")

	path := writeConfig(t, `
version: 1
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [slack_ops]
`)

	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestLoadRejectsMissingEnv(t *testing.T) {
	path := writeConfig(t, `
version: 1
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [slack_ops]
`)

	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "missing required environment variables") {
		t.Fatalf("expected missing env error, got %v", err)
	}
}

func TestLoadAcceptsDeprecatedStartupBackfill(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/services/abc")

	path := writeConfig(t, `
version: 1
startup_backfill: 10m
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [slack_ops]
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.StartupBackfill.Duration.String() != "10m0s" {
		t.Fatalf("StartupBackfill = %s", cfg.StartupBackfill)
	}
}

func TestLoadRejectsUnknownRouteChannelAndInvalidDuration(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/services/abc")

	path := writeConfig(t, `
version: 1
cooldown: nope
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [missing]
`)

	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid duration error, got %v", err)
	}
}

func TestLoadRejectsUnsupportedRouteNames(t *testing.T) {
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/services/abc")

	path := writeConfig(t, `
version: 1
channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
routes:
  default:
    send_to: [slack_ops]
  extra:
    send_to: [slack_ops]
`)

	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "field extra not found") {
		t.Fatalf("expected unsupported route error, got %v", err)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docknudge.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
