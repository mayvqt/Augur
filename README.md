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
`/approvals enable channel:#approvals` to post pending requests with Approve and Decline buttons, or
`/approvals disable` to turn those messages off. Use `/approvals status` to inspect the current setting. State is stored in `./data`.

Approval cards cover all Seerr request sources, notify linked requesters of the decision, and are removed two minutes
after approval or decline.

[Configuration](docs/configuration.md) · [Features](docs/features.md) · [Unraid](docs/unraid.md) · [Development](docs/development.md)
