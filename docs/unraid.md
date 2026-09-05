# Unraid

The template URL below follows `main`, so it is mutable and may change as Augur changes:

```sh
wget -O /boot/config/plugins/dockerMan/templates-user/my-augur.xml https://raw.githubusercontent.com/mayvqt/Augur/main/unraid/augur.xml
```

For a reproducible setup, save a template URL that points to a reviewed commit, and set the image repository to an
existing release tag such as `ghcr.io/mayvqt/augur:<version>`. Check the [published releases](https://github.com/mayvqt/Augur/releases)
for tags that actually exist; do not guess a future tag.

Open **Docker → Add Container → Augur**. Set the Discord token, Seerr API key, internal and public Seerr URLs, and appdata
path. The default appdata path is `/mnt/user/appdata/augur` mounted at `/data`.

Set a guild ID for faster command updates while testing. Leave it empty for global commands.
