# Documentation conventions

Write for the person using the page. Keep installation steps in
[Setup](../setup.md), settings in [Configuration](../configuration.md), command help in [Features](../features.md), shipped scope in
[Capabilities](../capabilities.md), release changes in [Releases](../releases.md),
and contributor details in this directory. Link instead of repeating a procedure.
Do not add empty roadmap or known-issues pages; add one only when there is accepted
future work or a reproduced issue worth publishing.

For a risky integration change, keep the agreed design stable while work is in
progress and record test evidence separately. List the external operations,
check Discord and Seerr behavior against their official documentation, cover it
with deterministic fakes, and make any live test clearly opt-in.

## Documentation impact map

| If you change... | Also update... |
| --- | --- |
| A command, message, approval, or notification | [Features](../features.md), [Capabilities](../capabilities.md) when scope changes, and the README if navigation changes |
| Configuration or setup | [Setup](../setup.md), [Configuration](../configuration.md), Unraid docs when affected, and their nearest index |
| Package ownership or architecture | [Codebase map](codebase-map.md), [Architecture](architecture.md), and this index |
| Schema, identifiers, or retained state | [Data](data.md), [Operations](operations.md) when upgrades change, and this index |
| Tests, tools, interaction states, or CI | [Validation](validation.md) and this index |
| Credentials, permissions, or disclosure | [Security](../../SECURITY.md), the relevant architecture/data/operations page, and the nearest index |
| Deployment, backup, health, rollback, or release | [Operations](operations.md), [Releases](../releases.md), and public setup/configuration pages that changed |

Add new documentation domains here and to the nearest index.
