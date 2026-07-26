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
| `worker.poll_interval` | How often Augur checks subscriptions. |
| `health.enabled` | Enable the optional HTTP health, readiness, and metrics server. |
| `health.address` | Listen address for the optional health server. |

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
| `AUGUR_HEALTH_ENABLED` | `true` or `false`. |
| `AUGUR_HEALTH_ADDRESS` | Optional listen address. Port `0` asks the OS to choose an unused port. |

## Storage

Augur stores request subscription state in SQLite. The default database filename is:

```text
augur-state.db
```

Containers use:

```text
/data/augur-state.db
```

SQLite WAL sidecar files may appear beside the database while Augur is running.
The schema uses a `subscriptions` table and supports multiple Discord subscribers
for the same Seerr request.

## Health And Metrics

When `health.enabled` is true, Augur serves:

| Path | Purpose |
| --- | --- |
| `/healthz` | Process liveness. |
| `/readyz` | Discord startup and storage-backed readiness. |
| `/metrics` | JSON counters for searches, requests, monitor checks, completions, failures, and retries. |

Health serving is disabled by default. Its default address is `127.0.0.1:0`, which
selects an unused local port and logs the chosen address at startup. Container users
who need to publish health endpoints must explicitly choose and publish a port.
