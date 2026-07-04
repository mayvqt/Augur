# Augur

Augur is a Discord bot for [Seerr](https://github.com/seerr-team/seerr).

Users run `/link` to open Seerr's Discord notification settings and copy their Discord ID64. After Seerr stores that ID in the user's notification settings, `/request` lets them fuzzy-search Seerr, pick a title from a dropdown, submit the request, and receive a DM when the request becomes available.

## Setup

1. Copy `config.example.json` to `config.json`.
2. Set `discord.token`.
3. Set `seer.base_url` and `seer.api_key`.
4. Set `link.public_url` to your public Seerr URL. `/link` appends `/profile/settings/notifications/discord`.
5. Users should paste their Discord ID64 into Seerr's Discord notification settings.
6. Run:

```sh
go run ./cmd/augur -config config.json
```

If `discord.guild_id` is empty, commands are registered globally. Setting it to a guild ID makes updates appear faster while testing.

## Environment Overrides

All important config values can be overridden:

- `AUGUR_CONFIG`
- `AUGUR_DISCORD_TOKEN`
- `AUGUR_GUILD_ID`
- `AUGUR_SEERR_BASE_URL`
- `AUGUR_SEERR_PUBLIC_URL`
- `AUGUR_SEERR_API_KEY`
- `AUGUR_LINK_PUBLIC_URL`
- `AUGUR_LINK_REQUIRE_MATCH`
- `AUGUR_STORAGE_PATH`
- `AUGUR_WORKER_POLL_INTERVAL`

The older `AUGUR_SEER_*` names are also accepted as aliases.

## Seerr Notes

Augur uses Seerr's API:

- `GET /api/v1/search?query=...`
- `GET /api/v1/user`
- `GET /api/v1/user/{id}/settings/notifications`
- `POST /api/v1/request`
- `GET /api/v1/request/{id}`

Requests are made with `X-Api-Key`. When `link.require_match` is true, Augur finds the Seerr user whose notification settings contain the Discord ID64 in `discordIds`, then submits requests with that Seerr `userId`.
