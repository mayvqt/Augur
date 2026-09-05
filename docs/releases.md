# Releases

`latest` is the rolling container tag and may change. Semver tags such as `v1.2.3` identify a specific release when that
tag is published. Pin production deployments to a published semver tag; use `latest` only when you accept rolling updates.

Release checklist:

1. Preflight: run the full checks in [`development.md`](development.md), review migration notes, and build and
   smoke-test the image.
2. Tag: create an annotated semver tag, for example `git tag -a v1.2.3 -m "v1.2.3"`.
3. Publish: push the tag and create the matching GitHub release, then verify the published container image.
4. Announce the release with upgrade notes and supported changes.

To roll back, stop the deployment, change the image to the previous known-good published tag, and start it again. Back up
`data/` before upgrades and keep the database with the deployment; do not edit or delete applied migrations.
