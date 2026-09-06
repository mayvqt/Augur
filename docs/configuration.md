# Configuration

Environment variables override `config.json`. Use `AUGUR_CONFIG` or `-config` to select another file.
The container creates `/data/config.json` from the example on first startup and
keeps it across restarts. When running the binary directly, create the file from
[`config.example.json`](../config.example.json) first.

## Required settings

- `AUGUR_DISCORD_TOKEN`
- `AUGUR_SEERR_API_KEY`
- `AUGUR_SEERR_BASE_URL`: URL reachable by Augur
- `AUGUR_SEERR_PUBLIC_URL`: URL opened by users

## Optional settings

- `AUGUR_GUILD_ID`: empty for global commands, or a Discord server ID for commands in that server
- `AUGUR_LINK_REQUIRE_MATCH`: `true`
- `AUGUR_STORAGE_PATH`: `/data/augur-state.db` in Docker
- `AUGUR_WORKER_POLL_INTERVAL`: `2m`
- `AUGUR_HEALTH_ENABLED`: `false`
- `AUGUR_HEALTH_ADDRESS`: `127.0.0.1:0`

See [`config.example.json`](../config.example.json) for the complete format.

Keep `AUGUR_LINK_REQUIRE_MATCH=true` for requests to use each person's Seerr
account. Setting it to `false` submits requests using the API key's identity
instead; it does not remove the linking requirement for `/requests`.

Containers also accept `PUID` and `PGID` (defaults `99` and `100`) for the user
and group that run Augur and own its persistent files. Both must be nonzero
numeric IDs.

The JSON file also controls settings that do not currently have environment-variable overrides:

- `discord.presence.enabled`: publish a Discord presence (`true` by default)
- `discord.presence.status`: `online`, `idle`, `dnd`, or `invisible`
- `discord.presence.type`: `playing`, `watching`, `listening`, or `competing`
- `discord.presence.message`: presence text shown in Discord
- `seer.timeout`: timeout for Seerr API calls (`15s` by default)

For approval setup and permission requirements, see [approval cards](features.md#approval-cards).

## Secrets and network access

`AUGUR_DISCORD_TOKEN_FILE` and `AUGUR_SEERR_API_KEY_FILE` can read secrets from mounted files. Do not set both forms of
the same secret. Keep secrets out of version control; prefer files in containers.

For Docker, `AUGUR_SEERR_BASE_URL` must resolve from inside the Augur container. A Compose service name such as
`http://seerr:5055` is valid only when Seerr is on the same Docker network. If Seerr runs elsewhere, use a reachable LAN
hostname or IP (for example `http://192.168.1.20:5055`). The public URL can be a different browser-facing address.

For file-based secrets, mount read-only files and point the variables at them, for example:

```yaml
environment:
  AUGUR_DISCORD_TOKEN_FILE: /run/secrets/discord-token
  AUGUR_SEERR_API_KEY_FILE: /run/secrets/seerr-api-key
```

Keep `.env`, secret files, and `data/` private. Back up the SQLite database in `data/` regularly and protect backups with
the same care as the API credentials; do not commit or publish them.

The optional server provides `/healthz`, `/readyz`, and `/metrics`. It has no authentication, so bind it to a private
interface or firewall it from the public internet.
