# DockNudge

DockNudge is a small Go service that watches Docker container events and sends focused alerts to Slack and Google Chat. It targets single-host Docker setups where you want failure visibility without a full monitoring stack.

## Features

- Docker event monitoring only
- Slack and Google Chat incoming webhooks
- Strict YAML config with `${ENV_VAR}` secret expansion
- Built-in alerts for `oom`, `die`, `health_status: unhealthy`, restart bursts, and best-effort unexpected stops
- `init`, `validate`, `test`, `run`, and `version` CLI commands

## Quick Start

```bash
go build -o docknudge ./cmd/docknudge
./docknudge init
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/..."
export GCHAT_WEBHOOK_URL="https://chat.googleapis.com/v1/spaces/..."
./docknudge validate -c docknudge.yml
./docknudge test -c docknudge.yml slack_ops
./docknudge run -c docknudge.yml
```

## Config

The default config path is `docknudge.yml`.

```yaml
version: 1
docker_host: unix:///var/run/docker.sock
cooldown: 10m
log_level: info

channels:
  slack_ops:
    type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
  gchat_ops:
    type: gchat
    webhook_url: ${GCHAT_WEBHOOK_URL}

routes:
  default:
    send_to: [slack_ops, gchat_ops]

rules:
  oom:
    enabled: true
  die:
    enabled: true
    ignore_exit_codes: [0, 143]
  unhealthy:
    enabled: true
  restart_burst:
    enabled: true
    threshold: 3
    window: 10m
  unexpected_stop:
    enabled: true
    lookback: 30s
```

## Commands

- `docknudge init [--force]` writes a starter config to `docknudge.yml`.
- `docknudge validate [-c path]` validates config and Docker connectivity.
- `docknudge test [-c path] <channel-name>` sends a sample alert to one channel.
- `docknudge run [-c path]` connects to the live Docker event stream and sends alerts for matching events.
- `docknudge version` prints build metadata.

## Development

Contributor setup and workflow live in [CONTRIBUTING.md](CONTRIBUTING.md).

Core local commands:

```bash
go test ./...
go build ./cmd/docknudge
./docknudge validate -c docknudge.yml
./docknudge test -c docknudge.yml slack_ops
```

GitHub Actions currently runs build and test only. Docker daemon checks and real webhook delivery remain manual validation steps.

## Runtime Model

- DockNudge is live-only: it does not replay historical Docker events on startup or reconnect.
- Docker stream disconnects are retried with exponential backoff, and a reconnect resumes from the next live event only.
- Duplicate alerts after a full process restart are still possible in V1 because state is kept only in memory.

## Unexpected Stop Classification

`unexpected_stop` is best-effort. DockNudge suppresses stop alerts when it has recent evidence that the stop was likely expected:

- `kill` with signal `15` or `9`
- `die` with exit code `0`, `137`, or `143`

This reduces common `docker stop` noise, but it does not guarantee perfect actor attribution.

## Docker Image

The included `Dockerfile` builds a small Linux image. Run it with the Docker socket and config mounted read-only:

```bash
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$(pwd)/docknudge.yml:/app/docknudge.yml:ro" \
  -e SLACK_WEBHOOK_URL \
  -e GCHAT_WEBHOOK_URL \
  docknudge:latest run
```
