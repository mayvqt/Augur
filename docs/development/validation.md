# Validation

Start with the narrowest useful check:

| Change | Focused check |
| --- | --- |
| Application coordination or health | `go test ./internal/app` |
| Discord commands, cards, or formatting | `go test ./internal/discordbot` |
| Seerr protocol | `go test ./internal/seer` |
| SQLite or migrations | `go test ./internal/storage` |
| Configuration or redaction | `go test ./internal/config ./internal/safelog` |
| CLI startup | `go test ./cmd/augur` |
| Release binary | `CGO_ENABLED=0 go build -trimpath -o /tmp/augur ./cmd/augur` |

Run `gofmt -w` on touched Go files. For performance work, include a
representative before/after benchmark or trace and a regression limit.

Discord changes also need a manual pass in a dedicated server with synthetic
requests. Check command discovery, keyboard use in Discord clients, ephemeral and
public responses, missing links, empty search/results, API errors, expired
components, permission denial, DM-disabled users, and shutdown during polling.
Check narrow and desktop Discord layouts when card content changes. Live Discord
or Seerr tests are opt-in and must never use production tokens or data.

Use the Go version in `go.mod`. Do not install tools or download dependencies
without approval. Reuse a module cache only when `go.sum`, the Go toolchain,
OS/architecture, and installed module tree match the revision; otherwise use a
clean locked environment or stop.

The final gate is the complete `CI` workflow in `.github/workflows/ci.yml` on
the exact revision. It checks formatting, `go mod tidy -diff`, whitespace,
tests, vet, the race detector, pinned Staticcheck and govulncheck versions, the
release build, Docker build, entrypoint behavior, and runtime ownership.
