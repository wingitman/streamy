# streamy

`streamy` is a terminal chat client for Twitch and YouTube.

## Run

Run `make build`, then `./streamy`. On first launch it creates:

```text
~/.config/delbysoft/streamy.toml
```

Use the in-app integrations setup by pressing `i`. It walks through provider
registration, accepts client credentials without putting secrets in TOML, and
stores the client secret in the OS keyring. You can also use
[`streamy.toml.example`](streamy.toml.example) as a starting point.

Install the generated binary with `make install`; use `INSTALL_DIR` to choose
the destination directory.

The TOML file contains connection identifiers and OAuth application IDs only.
Access tokens and client secrets are stored in the OS keyring under the
`streamy` service.

For a new connection, open integrations with `Ctrl+i` and choose a provider. The
guided setup stores client secrets in the OS keyring, not in TOML.

For YouTube, complete these steps:

1. In Google Cloud Console, select or create a project.
2. Enable **YouTube Data API v3** for that project.
3. Configure the OAuth consent screen and add your Google account as a test user
   if the app is still in testing.
4. Create an OAuth client and use the application type supported by Google for
   this local application. Register `http://localhost:43821/oauth/callback` if
   Google asks for an authorized redirect URI.
5. In Streamy's YouTube setup, enter a connection ID, a channel label, the
   client ID, and the client secret. Leave **live chat ID** blank to discover
   the active broadcast automatically. If multiple broadcasts are active, enter
   the desired chat ID manually.
7. Save the connection, then authorize it:
   `streamy --login youtube --connection <connection-id>`
8. Enable the connection after its live chat ID is present, then restart Streamy
   if it does not connect immediately.

The live chat ID identifies the chat attached to an active live broadcast; the
channel name alone is not sufficient. Streamy discovers it with
`liveBroadcasts.list` and reads `snippet.liveChatId`.

For Twitch, press the provider-console key, register the callback URL above, and
enter the connection and application values shown by the setup screen. The
connection initially remains disabled so provider-specific identifiers can be
added before enabling it.

The first launch with no configured connections is safe and opens an empty chat
view. Press `i` to configure an integration instead of editing TOML manually.

Keys: `1` combined, `2` Twitch, `3` YouTube, `Tab` changes the send target,
`Enter` sends, `r` retries the latest failed delivery, `c` reconnects the
selected connection, `/` filters, `o` edits the config, `T` selects a theme,
`H` opens history, `U` checks updates, `i` configures integrations, and `q` or
`Ctrl+C` quits.

Enabled connections need provider-specific identifiers in the TOML file:

- Twitch: `channel`, `broadcaster_id`, and `user_id`
- YouTube: `live_chat_id`

Before enabling a connection, add its provider-specific identifiers, then run
`streamy --login <platform> --connection <id>`. The OAuth application `client_id`
belongs in the TOML file; access and refresh tokens remain in the keyring.

## Verification

```text
make test
make vet
go test -race ./...
```
