# Augur

Discord bot for requesting movies and TV shows through [Seerr](https://github.com/seerr-team/seerr).

## Setup

Create a Discord bot with the `bot` and `applications.commands` scopes, then copy its token and a Seerr API key.

```sh
cp .env.example .env
# Add the token, API key, and Seerr URLs.
docker compose up -d
```

Use `/link` to connect a Discord account and `/request` to request media. State is stored in `./data`.

[Configuration](docs/configuration.md) · [Unraid](docs/unraid.md) · [Development](docs/development.md)
