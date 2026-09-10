# Codebase map

| Path | What lives here |
| --- | --- |
| `cmd/augur` | Process entry point, signal handling, configuration loading, and logging setup. |
| `internal/app` | Application lifecycle, request/approval coordination, polling, health endpoints, and metrics. |
| `internal/config` | JSON and environment configuration, defaults, normalization, and validation. |
| `internal/discordbot` | Discord session lifecycle, slash commands, buttons, previews, approval cards, formatting, and delivery cache. |
| `internal/seer` | Seerr HTTP client, account links, requests, status, and user lookup. |
| `internal/storage` | SQLite schema, migrations, subscriptions, approvals, notification preferences, and scans. |
| `internal/safelog` | Log redaction and safe error output. |
| `Dockerfile`, `docker-entrypoint.sh`, `docker-compose.yml` | Container build, startup, ownership, and example deployment. |
| `unraid` | Unraid application template. |
| `.github/workflows` | CI, dependency review, image publishing, and release announcements. |

Tests sit beside the packages they cover. Update this map when a package takes
on a new responsibility or a top-level area is added.
