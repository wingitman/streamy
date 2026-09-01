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

For a new connection, the guided setup:

1. Press `i`, choose a provider, and press `b` to open its developer console.
2. Register `http://localhost:43821/oauth/callback` when the provider asks for a
   redirect URI.
3. Enter the connection ID, channel, client ID, and client secret in Streamy.
4. Restart Streamy and run `streamy --login <platform> --connection <id>` to
   authorize it. The connection is initially disabled so you can add the
   provider-specific identifiers before enabling it.

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
