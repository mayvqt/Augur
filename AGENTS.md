# AGENTS.md

Guidance for coding agents working in this repository.

## Project Shape

Augur is a small Go service that connects Discord slash commands to Seerr requests, then watches accepted requests and sends Discord notifications when they become available.

Keep the code boring, explicit, and easy to scan. Prefer small files with one job over broad files that mix setup, workflow, formatting, transport, and persistence.

## Commands

Run these before handing off changes:

```bash
go test ./...
go vet ./...
```

If the default Go cache is not writable, use:

```bash
GOCACHE=/tmp/augur-go-build GOMODCACHE=/tmp/augur-go-mod go test ./...
```

## Package Boundaries

| Path | Owns | Avoid |
| --- | --- | --- |
| `cmd/augur` | Process startup, config loading, logging setup, and signal handling. | Business logic, Discord handlers, Seerr request details. |
| `internal/app` | Runtime wiring, workflows, watch polling, shutdown coordination. | Discord-specific UI details, SQL statements, HTTP response parsing. |
| `internal/config` | Config structs, defaults, environment overrides, validation. | Service startup or runtime side effects beyond environment reads. |
| `internal/discordbot` | Discord session lifecycle, slash commands, interactions, DMs, presence, selection cache. | Seerr API details, storage details, cross-service orchestration. |
| `internal/seer` | Seerr HTTP client, request/response models, API-specific errors. | Discord formatting, storage, polling loops. |
| `internal/storage` | SQLite schema and persistence operations. | Network calls, Discord calls, application workflow decisions. |

## File Organization

Use focused files named after their responsibility. As a rule of thumb, reconsider a file once it grows beyond about 250 lines or starts needing unrelated sections.

Current `internal/discordbot` layout:

- `bot.go`: core types and constructor.
- `lifecycle.go`: session start, close, ready handler, presence, outbound DMs.
- `commands.go`: slash command definitions and command/component IDs.
- `handlers.go`: interaction workflows for slash commands and components.
- `responses.go`: common Discord interaction response helpers.
- `format.go`: user-facing labels, truncation, interaction value extraction, link building.
- `cache.go`: short-lived selection cache for request pickers.

When adding a feature, place the code where the responsibility lives. If a file needs a second unrelated concern, make a new file in the same package.

## Maintainability Rules

- Keep public APIs narrow. Export only what other packages need.
- Keep handlers thin. Discord handlers should validate input, call the app workflow, then format the reply.
- Keep workflow decisions in `internal/app`, not in clients or storage.
- Keep clients context-aware. Any HTTP, Discord, or database operation should accept or be driven by a `context.Context`.
- Keep storage methods small and transactional only where needed.
- Avoid package cycles by depending inward on interfaces where useful.
- Prefer standard library features and existing local helpers before adding dependencies.
- Do not hide operational errors. Wrap errors with enough context at package boundaries.
- Do not log secrets, Discord tokens, Seerr API keys, or full config payloads.
- Do not make broad refactors while fixing narrow bugs unless the refactor directly reduces risk.

## Testing Expectations

- Add table-driven tests for validation, parsing, formatting, and pure helper logic.
- Add HTTP server tests for Seerr client behavior.
- Add storage tests for schema changes, persistence behavior, and edge cases.
- For Discord handlers, prefer testing pure helpers and workflow-facing interfaces instead of live Discord calls.
- Update tests with behavior changes. Do not only update snapshots or expected strings without checking the user-facing impact.

## Style

- Use `gofmt`.
- Keep comments useful and sparse. Explain why a decision exists, not what each line does.
- Prefer descriptive names over abbreviations except for common Go conventions like `ctx`, `cfg`, and `err`.
- Keep config keys and environment variables documented when adding them.
- Keep Docker, Unraid, and example config files in sync with config changes.
