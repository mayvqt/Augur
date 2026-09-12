# Data and migrations

`internal/storage/store.go` is the schema and migration source of truth. The
`schema_migrations` ledger records applied versions. Current tables store:

- request subscriptions and availability completion;
- approval settings per Discord server;
- approval-message IDs, delivery leases and retry deadlines;
- saved decision intents and confirmed decision notification jobs;
- cleanup of sent cards that could not be tracked;
- notification preferences per Discord user; and
- decision-notification deduplication.

Discord, guild, channel, message, Seerr request, and media IDs are identifiers,
not proof of authorization. Re-check permissions at the action boundary. The
database may reveal account relationships and request history, so treat it and
its backups as private.

Migrations run in transactions and move forward only. Add a new numbered
migration; never edit or delete a version that may already be applied. Prefer an
expand/contract sequence when a rolling transition needs old and new code to
coexist. Startup rejects a migration version newer than the binary understands
instead of risking changes to a future schema.

Each schema change needs tests for a fresh database, upgrade from every affected
supported version, preservation of existing rows, repeat startup, and failure
behavior. Keep SQL and migration compatibility in `internal/storage`. Restore
the full data directory for rollback; an older binary may not understand a newer
schema.

## Delivery state in revision 6

A claim token and two-minute lease fence each approval send. Retries honor the
current enabled channel, and acknowledgement/deletion of a card requires its
physical channel and message ID. Backoff grows from 30 seconds to 30 minutes.
Known untracked messages have separate durable cleanup records.

A decision intent preserves the moderator, reason and card presentation before
Seerr is called. An uncertain response is observed without replaying the write.
Confirmed decisions retain their first attribution and queue notification work
before Discord rendering. A single worker resolves current recipient links,
checks preferences, sends DMs and records successful or deliberately suppressed
recipients. Card removal does not remove notification work.

Discord delivery is at least once: a process crash after Discord accepts a message
but before SQLite records the receipt can cause a duplicate. Existing revision 5
receipts are retained; migration does not replay historical decisions. Legacy
blank card claims become recoverable. See [Operations](operations.md) before an
upgrade or rollback.
