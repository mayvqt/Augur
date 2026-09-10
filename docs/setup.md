# Setup

You need a Discord server where you can install an app, a reachable Seerr
instance, Docker with Compose, and a Seerr API key.

1. Create an application in the
   [Discord Developer Portal](https://discord.com/developers/applications) and
   add a bot. Install it with the `bot` and `applications.commands` scopes.
   Grant View Channel, Send Messages, Embed Links, and Manage Messages.
2. Clone the repository and open it:

   ```sh
   git clone https://github.com/mayvqt/Augur.git
   cd Augur
   ```

3. Copy the example environment file and add the Discord token, Seerr API key,
   internal Seerr URL, and public Seerr URL:

   ```sh
   cp .env.example .env
   $EDITOR .env
   ```

4. Start Augur and check its logs:

   ```sh
   docker compose up -d
   docker compose logs -f augur
   ```

5. Run `/link` in Discord. Open the Seerr settings link and save the Discord ID
   shown by the bot in your Seerr notification settings. Then try
   `/request query:<title>`.

`AUGUR_SEERR_BASE_URL` must be reachable from inside the Augur container. This
Compose file starts Augur only, so `http://seerr:5055` works only when a
container named `seerr` shares its Docker network. Otherwise, use the Seerr
host's LAN name or address. `AUGUR_SEERR_PUBLIC_URL` is the browser-facing URL
sent to users.

State is stored in `./data`. Before enabling approval cards, read the
[permissions and behavior](features.md#approval-cards). Before an upgrade, follow
the [backup and rollback guide](development/operations.md).
