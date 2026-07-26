# Configuration

Augur reads `config.json`. `AUGUR_CONFIG` or `-config` can select another file. Environment variables override values
from the file.

## Required environment variables

| Variable                 | Purpose                                |
|--------------------------|----------------------------------------|
| `AUGUR_DISCORD_TOKEN`    | Discord bot token.                     |
| `AUGUR_SEERR_BASE_URL`   | Internal Seerr URL reachable by Augur. |
| `AUGUR_SEERR_PUBLIC_URL` | Public Seerr URL used by `/link`.      |
| `AUGUR_SEERR_API_KEY`    | Seerr API key.                         |

Containers also need a writable `/data` mount. The default database path is
`/data/augur-state.db`.

## Optional environment variables

| Variable                     | Default                                 |
|------------------------------|-----------------------------------------|
| `AUGUR_GUILD_ID`             | Global commands                         |
| `AUGUR_LINK_REQUIRE_MATCH`   | `true`                                  |
| `AUGUR_STORAGE_PATH`         | `/data/augur-state.db` in the container |
| `AUGUR_WORKER_POLL_INTERVAL` | `2m`                                    |
| `AUGUR_HEALTH_ENABLED`       | `false`                                 |
| `AUGUR_HEALTH_ADDRESS`       | `127.0.0.1:0`                           |

The full file format is shown in
[`config.example.json`](../config.example.json).

## Health

The optional HTTP server provides `/healthz`, `/readyz`, and `/metrics`. It is disabled by default. Port `0` selects an
unused local port.
