# Configuration

Environment variables override `config.json`. Use `AUGUR_CONFIG` or `-config` to select another file.

Required:

- `AUGUR_DISCORD_TOKEN`
- `AUGUR_SEERR_API_KEY`
- `AUGUR_SEERR_BASE_URL`: URL reachable by Augur
- `AUGUR_SEERR_PUBLIC_URL`: URL opened by users

Optional:

- `AUGUR_GUILD_ID`: empty
- `AUGUR_LINK_REQUIRE_MATCH`: `true`
- `AUGUR_STORAGE_PATH`: `/data/augur-state.db` in Docker
- `AUGUR_WORKER_POLL_INTERVAL`: `2m`
- `AUGUR_HEALTH_ENABLED`: `false`
- `AUGUR_HEALTH_ADDRESS`: `127.0.0.1:0`

See [`config.example.json`](../config.example.json) for the complete format.

The JSON file also controls settings that do not currently have environment-variable overrides:

- `discord.presence.enabled`: publish a Discord presence (`true` by default)
- `discord.presence.status`: `online`, `idle`, `dnd`, or `invisible`
- `discord.presence.type`: `playing`, `watching`, `listening`, or `competing`
- `discord.presence.message`: presence text shown in Discord
- `seer.timeout`: timeout for Seerr API calls (`15s` by default)

Server administrators can run `/approvals status` to inspect the current approval-message setting. Use
`/approvals enable channel:#approvals` to enable it or `/approvals disable` to disable it. The bot needs View
Channel, Send Messages, Embed Links, and Manage Messages permissions in the selected channel.

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
