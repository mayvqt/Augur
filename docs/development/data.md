# Data and migrations

`internal/storage/store.go` is the schema and migration source of truth. The
`schema_migrations` ledger records applied versions. Current tables store:

- request subscriptions and availability completion;
- approval settings per Discord server;
- approval-message IDs and decisions;
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
