# Development

Augur is a small Go service with explicit package boundaries.

## Commands

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/augur
```

If the default Go cache is not writable in your environment, use writable caches:

```bash
GOCACHE=/tmp/augur-go-build GOMODCACHE=/tmp/augur-go-mod go test ./...
```

## Docker

```bash
docker compose up --build
```

The container stores config and the SQLite database under `/data`.

## Layout

| Path | Purpose |
| --- | --- |
| `cmd/augur` | Process startup, config loading, and signal handling. |
| `internal/app` | Runtime wiring, request creation, watch polling, and shutdown. |
| `internal/config` | Config file parsing, environment overrides, and validation. |
| `internal/discordbot` | Discord slash commands, interactions, DMs, presence, and selection cache. |
| `internal/seer` | Seerr API client. |
| `internal/storage` | SQLite persistence for watched requests. |

## Notes

- Keep network calls outside storage transactions.
- Keep Discord handlers small and push workflow logic into `internal/app`.
- Keep the Seerr client context-aware and response-size bounded.
