# Augur

Discord bot for requesting movies and TV shows through [Seerr](https://github.com/seerr-team/seerr).

## Quick start

You need a Discord server where you can install an app, a reachable Seerr instance, Docker with Compose, and a Seerr API
key. Create the key in Seerr's administration/settings area; the exact menu label can vary between Seerr releases.

1. In the [Discord Developer Portal](https://discord.com/developers/applications), create an application and add a bot.
   Install it with the `bot` and `applications.commands` scopes. Grant the bot View Channel, Send Messages, Embed Links,
   and Manage Messages; administrators also need Manage Server to configure approval messages.
2. Copy the example environment file and fill in the Discord token, Seerr API key, Seerr URLs, and any optional settings:

   ```sh
   cp .env.example .env
   $EDITOR .env
   ```

3. Start Augur:

   ```sh
   docker compose up -d
   ```

4. Verify it with `docker compose logs -f augur`, then use `/link` in Discord. After linking, `/request` searches Seerr
   and submits a request. Server administrators can use `/approvals enable channel:#approvals` for approval cards.

`AUGUR_SEERR_BASE_URL` must be reachable from the Augur container. The sample Seerr URL is intentionally a host
placeholder: this Compose file starts Augur only, so `http://seerr:5055` works only when a container named `seerr` is on
the same Docker network. For a separate Seerr installation, use its LAN hostname or IP and port instead. The public URL
is the browser-facing address that Augur sends to users for account linking.

Approval cards cover all Seerr request sources, notify linked requesters of the decision, and are removed two minutes
after approval or decline. State is stored in `./data`.

## Documentation

[Configuration](docs/configuration.md) · [Features](docs/features.md) · [Unraid](docs/unraid.md) ·
[Releases](docs/releases.md) · [Security](SECURITY.md) · [Development](docs/development.md) · [License](LICENSE) ·
[Issues](https://github.com/mayvqt/Augur/issues)
