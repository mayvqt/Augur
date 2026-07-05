# Setup

Augur needs a Discord bot token, a Seerr API key, and a public Seerr URL for the `/link` flow.

## Discord

1. Create a Discord application and bot in the Discord Developer Portal.
2. Copy the bot token into `discord.token` or `AUGUR_DISCORD_TOKEN`.
3. Invite the bot with the `bot` and `applications.commands` scopes.
4. Set `discord.guild_id` while testing so slash command updates appear quickly.

Leave `discord.guild_id` empty for global commands once the bot is ready.

## Seerr

1. Create or copy a Seerr API key.
2. Set `seer.base_url` to the internal Seerr URL Augur can reach.
3. Set `link.public_url` to the browser-facing Seerr URL users open.

`/link` sends users to:

```text
{link.public_url}/profile/settings/notifications/discord
```

Users paste their Discord ID64 there. When `link.require_match` is enabled, Augur only allows `/request` after it finds that Discord ID64 in the user's Seerr notification settings.

## Run Locally

```bash
cp config.example.json config.json
# edit config.json
go run ./cmd/augur -config config.json
```

## Commands

- `/link`: replies privately with the user's Discord ID64 and a Seerr settings link.
- `/request`: searches Seerr, shows up to 25 movie/show matches, and submits the selected request.

Augur watches submitted requests and DMs the requester when Seerr reports the media as available.
