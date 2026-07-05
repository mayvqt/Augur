# Unraid

Augur is a headless Discord bot for Seerr. It does not expose a web UI; all interaction happens through Discord slash commands and your Seerr web UI.

## Install Template

Run from an Unraid terminal:

```bash
wget -O /boot/config/plugins/dockerMan/templates-user/my-augur.xml https://raw.githubusercontent.com/mayvqt/Augur/main/unraid/augur.xml
```

Then open `Docker` > `Add Container`, choose `Augur`, fill required values, and apply.

## Required Values

| Name | Value |
| --- | --- |
| `AUGUR_DISCORD_TOKEN` | Discord bot token. |
| `AUGUR_SEERR_BASE_URL` | Internal Seerr URL reachable from the container, for example `http://seerr:5055`. |
| `AUGUR_SEERR_PUBLIC_URL` | Browser-facing Seerr URL, for example `https://seerr.example.com`. |
| `AUGUR_SEERR_API_KEY` | Seerr API key. |
| `/data` | `/mnt/user/appdata/augur` mounted read/write. |

Set `AUGUR_GUILD_ID` while testing so slash commands update quickly. Leave it blank for global commands once you are happy.

## Link Flow

When a user runs `/link`, Augur replies privately with:

- Their Discord ID64.
- A button to `AUGUR_SEERR_PUBLIC_URL/profile/settings/notifications/discord`.

Users paste the ID64 into Seerr. Augur then checks each user's Seerr notification settings and looks for that value in `discordIds` before allowing `/request`.

## Container Settings

| Setting | Value |
| --- | --- |
| Image | `ghcr.io/mayvqt/augur:latest` |
| AppData path | `/mnt/user/appdata/augur` -> `/data` |
| Network | `bridge`, unless your Seerr stack needs a custom Docker network. |
| PUID / PGID | Unraid defaults are `99` / `100`. |

Augur stores request watch state in `/data/augur-state.db` using SQLite with WAL journaling.

## Quick Checks

- Slash commands missing: set `AUGUR_GUILD_ID`, restart Augur, and invite the bot with `applications.commands`.
- `/link` opens the wrong host: fix `AUGUR_SEERR_PUBLIC_URL`.
- `/request` says the account is not linked: verify the user pasted the Discord ID64 into Seerr's Discord notification settings.
- Completion DMs missing: the user may have DMs disabled for the server.
