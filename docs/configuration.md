# Configuration

Augur reads `config.json` by default. Use `-config` or `AUGUR_CONFIG` to choose another file.

## Config File

Start from [config.example.json](../config.example.json).

| Field | Purpose |
| --- | --- |
| `discord.token` | Discord bot token. |
| `discord.guild_id` | Optional Discord server ID for faster slash command updates while testing. |
| `discord.presence` | Optional Discord presence settings. |
| `seer.base_url` | Internal Seerr URL reachable by Augur. |
| `seer.api_key` | Seerr API key. |
| `seer.timeout` | Per-request Seerr HTTP timeout. |
| `link.public_url` | Browser-facing Seerr URL used by `/link`. |
| `link.require_match` | Require a matching Seerr Discord notification ID before allowing `/request`. |
| `storage.path` | SQLite database path. |
| `worker.poll_interval` | How often Augur checks watched requests. |

## Environment

Environment variables override matching file values:

| Variable | Purpose |
| --- | --- |
| `AUGUR_CONFIG` | Config file path. |
| `AUGUR_DISCORD_TOKEN` | Discord bot token. |
| `AUGUR_GUILD_ID` | Optional Discord guild/server ID. |
| `AUGUR_SEERR_BASE_URL` | Internal Seerr URL. |
| `AUGUR_SEERR_PUBLIC_URL` | Browser-facing Seerr URL. |
| `AUGUR_SEERR_API_KEY` | Seerr API key. |
| `AUGUR_LINK_REQUIRE_MATCH` | `true` or `false`. |
| `AUGUR_STORAGE_PATH` | SQLite database path. |
| `AUGUR_WORKER_POLL_INTERVAL` | Duration such as `30s`, `2m`, or `5m`. |

## Storage

Augur stores request watch state in SQLite. New configs use:

```text
augur-state.db
```

Containers use:

```text
/data/augur-state.db
```

SQLite WAL sidecar files may appear beside the database while Augur is running.
