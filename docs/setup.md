# Setup

Augur needs a Discord bot token, a Seerr API key, and URLs for reaching Seerr.

## Discord

1. Create an application and bot in the [Discord Developer Portal](https://discord.com/developers/applications).
2. Invite it with the `bot` and `applications.commands` scopes.
3. Copy its token.
4. Optionally set a guild ID while testing so command changes appear immediately.

## Seerr

1. Copy an API key from Seerr.
2. Choose the internal Seerr URL Augur can reach.
3. Choose the public Seerr URL users open in their browser.

The `/link` command directs users to Seerr's Discord notification settings, where they add their Discord ID64. Linked
users can then search and submit requests with
`/request`.

## Docker

Set the [required environment variables](configuration.md#required-environment-variables), then start Augur:

```bash
docker compose up -d
```

The container creates `/data/config.json` automatically on first run.

## Local

```bash
cp config.example.json config.json
# Edit config.json.
go run ./cmd/augur -config config.json
```
