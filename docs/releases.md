# Releases

`latest` is the rolling container tag and may change. Semver tags such as `v1.2.3` identify a specific release when that
tag is published. Pin production deployments to a published semver tag; use `latest` only when you accept rolling updates.

## Unreleased

- Standardize project documentation and harden release automation.

- Reject zero-padded root user/group IDs in container configuration.
- Clarify setup, account linking, approval permissions, and backup procedures.

Release and upgrade procedures live in [Operations](development/operations.md).
This page is only for changes that shipped or are ready for the next release.
