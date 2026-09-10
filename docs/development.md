# Development

Start with the [development task index](development/README.md). It links to the
code map, architecture, storage, validation, operations, and documentation
guides.

For a manual integration test, follow the [setup guide](setup.md)
with a dedicated Discord application and server, a test Seerr instance, and an
isolated data directory. `docker compose up --build` starts a real bot and uses
the secrets in your local environment; stop it with `docker compose down` when
you finish.
