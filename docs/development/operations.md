# Operations

Run Augur with the smallest practical permissions: one writable `/data`
mount, outbound access to Discord and Seerr, and no public inbound port unless
the optional health server is deliberately exposed to a private monitoring
network. Keep the Discord token, Seerr API key, `.env`, `config.json`, data
directory, and diagnostics private. Redact IDs and request details when they are
not needed to diagnose a problem.

## Back up, upgrade, and roll back

1. Record the current immutable image tag or digest and read the release notes.
2. Stop Augur, then copy the entire data directory and deployment configuration
   to restricted off-host storage.
3. Restore that backup into a separate test location and confirm the bot starts,
   retains approval settings and subscriptions, and can read the database.
4. Deploy a published version tag or digest, then check logs, a harmless command,
   a synthetic request, and any configured approval channel.

`latest` moves and is not a rollback reference. Migrations run at startup and
are forward-only. To roll back across a schema change, stop Augur, restore the
matching pre-upgrade data, and start the previous immutable image. Local state
created after the backup is lost; requests already sent to Seerr are not undone.

When enabled, `/healthz` reports that the health process is running, `/readyz`
checks readiness and SQLite access, and `/metrics` exposes counters. These
endpoints are unauthenticated, so bind them privately. A release is complete only
after the exact revision passes CI, the published artifact is tied to that
revision, and post-deploy health and Discord/Seerr checks succeed.

The release sequence is: update [release notes](../releases.md), run the final CI
gate, create an annotated `vX.Y.Z` tag, publish the matching GitHub release,
verify the versioned image, and announce any operator action. Repairs that change
Discord messages, Seerr requests, or SQLite state must be explicit and opt-in.
