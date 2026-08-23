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

`AUGUR_DISCORD_TOKEN_FILE` and `AUGUR_SEERR_API_KEY_FILE` can read secrets from mounted files. Do not set both forms of
the same secret. Keep secrets out of version control; prefer files in containers.

The optional server provides `/healthz`, `/readyz`, and `/metrics`. It has no authentication; keep it private.
