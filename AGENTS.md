# AGENTS.md

Guidance for coding agents working in this repository.

## Mission

Augur is a small Go service that connects Discord slash commands to Seerr requests, watches accepted requests, and sends Discord notifications when media becomes available.

Keep the code boring, explicit, hardened, and easy to scan. Prefer small files with one job over broad files that mix setup, workflow, formatting, transport, persistence, and validation.

## Before You Change Code

- Read the files that own the behavior before editing.
- Check `git status --short` and do not overwrite unrelated user changes.
- Keep changes scoped to the requested behavior.
- Prefer standard library features and existing local helpers before adding dependencies.
- Do not introduce package cycles.
- Do not log secrets, Discord tokens, Seerr API keys, raw config payloads, or full request bodies.
- Remove local build artifacts such as `./augur` before handoff.

## Commands

Run these before handing off code changes:

```bash
go test ./...
go vet ./...
```

If the default Go cache is not writable, use writable caches:

```bash
GOCACHE=/tmp/augur-go-build GOMODCACHE=/tmp/augur-go-mod go test ./...
GOCACHE=/tmp/augur-go-build GOMODCACHE=/tmp/augur-go-mod go vet ./...
```

For build-only checks:

```bash
GOCACHE=/tmp/augur-go-build GOMODCACHE=/tmp/augur-go-mod CGO_ENABLED=0 go build ./cmd/augur
```

## Package Boundaries

| Path | Owns | Avoid |
| --- | --- | --- |
| `cmd/augur` | Process startup, flags, config loading, logging setup, signal handling, exit codes. | Business logic, Discord handlers, Seerr request details, SQL. |
| `internal/app` | Runtime wiring, workflow decisions, request creation flow, watch polling, shutdown coordination. | Discord UI details, SQL statements, raw HTTP parsing. |
| `internal/config` | Config structs, defaults, environment overrides, normalization, validation. | Service startup side effects beyond environment reads. |
| `internal/discordbot` | Discord session lifecycle, slash commands, interactions, DMs, presence, selection cache, Discord message formatting. | Seerr API details, storage details, cross-service orchestration. |
| `internal/seer` | Seerr HTTP client, request/response models, API validation, API-specific errors, availability status helpers. | Discord formatting, storage, polling loops. |
| `internal/storage` | SQLite path handling, schema, pragmas, row scanning, persistence operations, timestamp serialization. | Network calls, Discord calls, application workflow decisions. |

When a behavior spans packages, put the decision in `internal/app` and keep clients/storage as narrow adapters.

## File Organization

Use focused files named after their responsibility. Reconsider a file once it grows beyond about 250 lines, or sooner if it has unrelated sections.

Current package layout:

- `cmd/augur/main.go`: startup and process-level error handling.
- `internal/app/runner.go`: app wiring and lifecycle.
- `internal/app/interfaces.go`: app-facing interfaces for Seerr, storage, and notifications.
- `internal/app/requests.go`: request/search workflow decisions and duplicate-watch policy.
- `internal/app/watcher.go`: watch polling, retry, completion, and notification flow.
- `internal/app/health_server.go`: optional health, readiness, and JSON metrics server.
- `internal/app/metrics.go`: runtime counters exposed through `/metrics`.
- `internal/config/types.go`: config model.
- `internal/config/load.go`: config loading, defaults, and normalization.
- `internal/config/env.go`: environment overrides.
- `internal/config/validate.go`: config validation helpers.
- `internal/config/duration.go`: JSON duration parsing and formatting.
- `internal/discordbot/bot.go`: core Discord bot type and constructor.
- `internal/discordbot/lifecycle.go`: session start, close, ready handler, presence, outbound DMs.
- `internal/discordbot/commands.go`: slash command definitions and command/component IDs.
- `internal/discordbot/handlers.go`: Discord interaction workflows.
- `internal/discordbot/responses.go`: Discord interaction response helpers.
- `internal/discordbot/format.go`: labels, truncation, interaction value extraction, link building.
- `internal/discordbot/cache.go`: short-lived selection cache for request pickers.
- `internal/seer/client.go`: HTTP client, endpoint methods, request construction, response handling.
- `internal/seer/status.go`: notification-setting matching and availability status interpretation.
- `internal/storage/store.go`: store lifecycle, schema initialization, watch persistence methods.
- `internal/storage/path.go`: storage path normalization and directory creation.
- `internal/storage/scan.go`: SQL row scanning and scan-time parse errors.
- `internal/storage/time.go`: timestamp formatting and parsing.

If a file needs a second unrelated concern, make a new file in the same package.

Prefer file names that describe the domain behavior, not the implementation technique. For example, use `requests.go`, `watcher.go`, or `health_server.go` over vague names like `logic.go`, `helpers.go`, or `manager.go`.

## Hardening Rules

- Validate and normalize config at the edge before starting services.
- Trim user-controlled string fields before validation or persistence.
- Reject impossible IDs and unsupported media types before calling external systems.
- Keep HTTP, Discord, and database operations context-aware.
- Bound remote response bodies and error snippets.
- Wrap errors with package-boundary context, but do not include secrets.
- Keep network calls outside storage transactions.
- Keep SQLite transactions short and single-purpose.
- Make shutdown idempotent. Closing twice should be safe.
- Keep shutdown bounded. If a goroutine can block on external I/O, it must observe context cancellation or have a timeout.
- Prefer ephemeral Discord responses for user-specific or failure messages.
- Set `AllowedMentions` on Discord messages unless mentions are intentionally needed.
- Treat external API shapes as unstable: decode defensively and test edge cases.
- Keep health and metrics endpoints non-secret. Expose counters and readiness, not config values or request payloads.
- Bind health endpoints narrowly by default. Use localhost for examples unless container networking requires `0.0.0.0`.

## Maintainability Rules

- Keep public APIs narrow. Export only what another package needs.
- Keep handlers thin. Validate input, call the app workflow, format the reply.
- Keep workflow decisions in `internal/app`, not in clients or storage.
- Keep storage methods small and transactional only where needed.
- Use interfaces only when they reduce coupling or make tests cleaner.
- Put app-owned interfaces in `internal/app`; do not export them unless another package truly needs them.
- Keep metrics names stable once documented. Add new counters rather than renaming existing ones casually.
- Avoid broad refactors while fixing narrow bugs unless the refactor directly reduces risk.
- Prefer table-driven tests for pure logic and validation.
- Avoid clever abstractions for code that is already short and clear.

## Testing Expectations

- Add tests with behavior changes.
- Add table-driven tests for validation, normalization, parsing, formatting, and pure helper logic.
- Add HTTP transport or test-server tests for Seerr client behavior.
- Add storage tests for schema changes, persistence behavior, validation, and edge cases.
- Add app workflow tests with fakes for Seerr, storage, and notifier behavior.
- Add health handler tests when changing `/healthz`, `/readyz`, or `/metrics`.
- For Discord handlers, prefer testing pure helpers and workflow-facing interfaces instead of live Discord calls.
- Include regression tests for bugs you fix.
- Do not update expected strings blindly. Check the user-facing impact.

## Config And Docs

When changing config:

- Update `config.example.json`.
- Update `config.docker.json` when container defaults change.
- Update `docs/configuration.md`.
- Update Docker Compose or Unraid templates if environment variables or mounts change.
- Keep environment variable names stable unless there is a clear migration reason.
- Keep Docker defaults and bare-metal examples intentionally different when needed. Container health can listen on `0.0.0.0`, while local examples should prefer `127.0.0.1`.

## Style

- Use `gofmt`.
- Use clear names over abbreviations except common Go conventions like `ctx`, `cfg`, `db`, and `err`.
- Keep comments useful and sparse. Explain why a decision exists, not what each line does.
- Prefer explicit error messages that help operators fix config or runtime failures.
- Keep Markdown plain and readable.

## Handoff Checklist

Before final response:

- Run `gofmt` on touched Go files.
- Run `go test ./...` and `go vet ./...`, using `/tmp` caches if needed.
- Run `CGO_ENABLED=0 go build ./cmd/augur` for runtime or packaging changes.
- Remove generated binaries or temporary files created by verification.
- Check `git status --short`.
- Mention any commands that could not be run and why.
- Summarize the behavioral impact, not every mechanical edit.
