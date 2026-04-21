# Contributing to DockNudge

DockNudge uses a small, trunk-based workflow. Keep changes focused, validate them locally, and open pull requests against `main`.

## Prerequisites

- Go 1.25
- Docker Engine for manual runtime validation
- Colima is fine on macOS if you point `docker_host` at the Colima socket
- Optional webhook env vars for local alert tests:
  - `SLACK_WEBHOOK_URL`
  - `GCHAT_WEBHOOK_URL`

## Local Development

Build and test from the repository root:

```bash
go test ./...
go build ./cmd/docknudge
```

Useful local commands:

```bash
./docknudge init
./docknudge validate -c docknudge.yml
./docknudge test -c docknudge.yml slack_ops
./docknudge run -c docknudge.yml
```

Notes:

- `docknudge test` expects flags before the channel name.
- `validate` only checks config loading and Docker connectivity.
- Runtime and webhook behavior are still best validated manually against a real Docker daemon and webhook target.

## Branch Strategy

DockNudge uses `main` as the default branch.

- Branch from `main`
- Use short-lived branches such as `feat/...`, `fix/...`, or `docs/...`
- Open pull requests back into `main`
- Prefer squash merges to keep history compact
- Cut releases and tags from `main`

Avoid long-lived integration branches unless there is a temporary release need.

## Pull Requests

Before opening a PR:

- run `go test ./...`
- run `go build ./cmd/docknudge`
- update docs for any user-facing config, CLI, or behavior changes
- include manual validation notes when the change affects Docker integration or webhook delivery

PRs should stay focused. If a change is unrelated, split it into a separate PR.

## Repo Hygiene

Do not commit local-only artifacts such as:

- the `docknudge` binary
- root `.graphify_*` temp files
- `graphify-out/cache/`
- `graphify-out/.graphify_python`
- `graphify-out/.graphify_chunk_*.json`

Published graph artifacts under `graphify-out/` are kept for documentation, but the temporary and cache files are intentionally ignored.
