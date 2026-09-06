# Development

Use the Go version declared in [`go.mod`](../go.mod). Run automated checks from
the repository root:

```sh
gofmt -l ./cmd ./internal
go mod tidy -diff
go test ./...
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./cmd/augur
```

The formatting command should print no paths. Tests use temporary SQLite files
and fake clients/transports; they do not need live Discord or Seerr credentials.
[CI](../.github/workflows/ci.yml) also runs static analysis, dependency vulnerability
checks, and a container build with entrypoint smoke tests. Image publishing reuses
these same gates. CI pins action commits and analysis tool versions; update the
pins deliberately and validate their checks before release.

For manual integration testing, follow the [setup instructions](../README.md#quick-start)
with a dedicated Discord application/server, Seerr instance, and isolated data.
`docker compose up --build` starts a live bot, reads local configuration/secrets,
and mounts `./data`; it is not an automated test. Stop the test deployment with
`docker compose down` when finished.
