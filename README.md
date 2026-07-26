# Augur

Augur is a lightweight Discord bot for requesting movies and TV shows through
[Seerr](https://github.com/seerr-team/seerr).

Users can link their Seerr account, search with `/request`, review media details before submitting, and receive a DM
when their request becomes available.

## Documentation

- [Setup](docs/setup.md)
- [Configuration](docs/configuration.md)
- [Unraid](docs/unraid.md)
- [Development](docs/development.md)

## Run with Docker

Set the [required environment variables](docs/configuration.md#required-environment-variables), then run:

```bash
docker compose up -d
```

Augur stores its generated configuration and SQLite database in `./data`.
