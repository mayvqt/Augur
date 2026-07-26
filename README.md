# Augur

Augur is a headless Discord bot for [Seerr](https://github.com/seerr-team/seerr).

It lets Discord users link their Seerr account, search movies and shows with `/request`, submit requests from a dropdown, and receive a DM when the requested media becomes available.

## Quick Start

```bash
cp config.example.json config.json
# edit config.json
go run ./cmd/augur -config config.json
```

## Docs

- [Setup](docs/setup.md)
- [Configuration](docs/configuration.md)
- [Unraid](docs/unraid.md)
- [Development](docs/development.md)

## Checks

```bash
go test ./...
go vet ./...
```
