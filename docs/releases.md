# Releases

`latest` is the rolling container tag and may change. Semver tags such as `v1.2.3` identify a specific release when that
tag is published. Pin production deployments to a published semver tag; use `latest` only when you accept rolling updates.

## Unreleased

- Reject zero-padded root user/group IDs in container configuration.
- Clarify setup, account linking, approval permissions, and backup procedures.

## Release procedure

Release checklist:

1. Preflight: run the full checks in [`development.md`](development.md), review migration notes, and build and
   smoke-test the image.
2. Tag: create an annotated semver tag, for example `git tag -a v1.2.3 -m "v1.2.3"`.
3. Publish: push the tag and create the matching GitHub release, then verify the published container image.
4. Announce the release with upgrade notes and supported changes.

## Upgrading and rolling back

1. Read the release notes and record the image tag you currently use. Choose an
   image tag listed on the [published release](https://github.com/mayvqt/Augur/releases);
   Git tags include a `v` prefix, while versioned container tags omit it.
2. Stop Augur with `docker compose stop augur`, then back up the entire `data/`
   directory and your deployment configuration. Keep the backup private: it
   contains account IDs, request state, and possibly credentials.
3. Set `image:` in `docker-compose.yml` to the chosen container tag, run
   `docker compose pull augur`, then `docker compose up -d augur`.
4. Check `docker compose logs --tail=100 augur`, then verify `/request` and any
   approval channel you use.

Database migrations run on startup. Do not edit or delete applied migrations.
An older image may not support a newer database schema. To roll back, stop Augur,
restore the matching pre-upgrade data backup when required by the release notes,
and start the previous image tag. Restoring a backup loses local changes made
since that backup; Seerr requests already submitted are not undone.
