# Development

Requires the Go version declared in [`go.mod`](../go.mod).

```bash
go test ./...
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./cmd/augur
```

Build and run the container with:

```bash
docker compose up --build
```

Code lives under `internal/`, grouped by application workflow, configuration, Discord, Seerr, and SQLite storage.
