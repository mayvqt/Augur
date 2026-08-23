# Augur

Discord bot for requesting movies and TV shows through [Seerr](https://github.com/seerr-team/seerr).

## Setup

Create a Discord bot with the `bot` and `applications.commands` scopes, then copy its token and a Seerr API key.

```sh
cp .env.example .env
# Add the token, API key, and Seerr URLs.
docker compose up -d
```

Use `/link` to connect a Discord account and `/request` to request media. A server administrator can run
`/setup enabled:true channel:#approvals` to post pending requests with Approve and Decline buttons, or
`/setup enabled:false` to turn those messages off. State is stored in `./data`.

Approval messages cover pending requests created in Augur, the Seerr website, or another API client. Augur reconciles
Seerr's pending queue on startup and every worker polling interval, using the Seerr request ID to prevent duplicates.

[Configuration](docs/configuration.md) · [Unraid](docs/unraid.md) · [Development](docs/development.md)
