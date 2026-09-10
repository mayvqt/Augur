# Architecture

`cmd/augur` loads configuration and creates an `app.Runner`. The runner owns
the Discord connection, Seerr client, SQLite store, optional health server, and
background monitor. Discord handlers ask the runner to perform application
operations; protocol details stay in `internal/discordbot` and `internal/seer`,
and durable state stays in `internal/storage`.

## Sources of truth

- Discord owns server membership, channel permissions, interaction identity,
  messages, and component events.
- Seerr owns linked users, request permissions, quotas, request decisions, and
  media availability.
- SQLite owns Augur's subscriptions, delivery decisions, approval-channel
  settings, rendered-message references, and notification preferences.
- `config.json` supplies defaults; environment variables override supported
  fields at startup. [Configuration](../configuration.md) is the public contract.

Commands and component IDs are public interaction contracts. Seerr endpoints and
Discord payloads are external contracts: check their current official
documentation, then cover changes with fake clients or transports before using a
dedicated live test environment.

## Trust and concurrency

The Discord token, Seerr API key, user IDs, guild/channel IDs, request details,
and database are sensitive. Approval actions use the configured Seerr API key,
so the Manage Server or Administrator permission check is a security boundary.
Account-name matching must never replace explicit Seerr linking.

The runner serializes lifecycle changes and approval reconciliation separately.
Background polling, Discord callbacks, and shutdown can overlap. Keep operations
idempotent, make state changes durable before sending notifications where
practical, bound retries and waits, and honor context cancellation. SQLite uses
one connection and WAL mode; do not add parallel database owners.
