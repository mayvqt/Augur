# Unraid

```sh
wget -O /boot/config/plugins/dockerMan/templates-user/my-augur.xml https://raw.githubusercontent.com/mayvqt/Augur/main/unraid/augur.xml
```

Open **Docker → Add Container → Augur**. Set the Discord token, Seerr API key, internal and public Seerr URLs, and appdata
path. The default appdata path is `/mnt/user/appdata/augur` mounted at `/data`.

Set a guild ID for faster command updates while testing. Leave it empty for global commands.
