# Unraid

Install the template from an Unraid terminal:

```bash
wget -O /boot/config/plugins/dockerMan/templates-user/my-augur.xml https://raw.githubusercontent.com/mayvqt/Augur/main/unraid/augur.xml
```

Open **Docker → Add Container**, select **Augur**, and provide:

- Discord bot token
- Seerr internal URL
- Seerr public URL
- Seerr API key
- A writable appdata mapping, normally `/mnt/user/appdata/augur` → `/data`

The container creates `/data/config.json` and `/data/augur-state.db`
automatically. The default Unraid user and group IDs are `99` and `100`.

Set a guild ID while testing for immediate slash-command updates. Leave it empty to register commands globally.

## Troubleshooting

- Missing commands: invite the bot with `applications.commands`, set a guild ID, and restart Augur.
- Incorrect `/link` URL: check the public Seerr URL.
- Account not linked: add the Discord ID64 shown by `/link` to the user's Seerr Discord notification settings.
- Missing completion DM: allow direct messages from server members.
