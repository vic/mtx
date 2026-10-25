<p align="right">
  <a href="https://github.com/sponsors/vic"><img src="https://img.shields.io/badge/sponsor-vic-white?logo=githubsponsors&logoColor=white&labelColor=%23FF0000" alt="Sponsor Vic"/></a>
  <a href="https://github.com/vic/mtx/releases"><img src="https://img.shields.io/github/v/release/vic/mtx?style=plastic&logo=github&color=purple"/></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/vic/mtx" alt="License"/></a>
</p>

# mtx - a terminal Matrix client

mtx is a terminal client for Matrix. Rooms are channels (`&name`). It uses end-to-end
encryption by default, and threads, replies, edits, read markers and typing are shown
in the TUI.

The TUI is inspired by [ircc](https://github.com/vic/ircc), an IRC-like chat for
Claude Code sessions. mtx keeps its layout, keys and commands and replaces the backend
with a Matrix client built on [mautrix/go](https://github.com/mautrix/go).

## Build

```sh
nix-shell -p go --run 'go build -tags goolm -o mtx ./cmd/mtx'
just build          # the same, from the justfile
```

The `goolm` build tag selects the pure-Go Olm implementation. The SQLite store uses
`modernc.org/sqlite`, so the binary builds with `CGO_ENABLED=0`.

## Use

Start `mtx` with no arguments.

**`mtx login`:** asks you to type `yes`, deletes the store, and runs the first-run
screen again. Lost: E2EE keys and sync state. The old device stays on the server
until you remove it there.

Once you logged store `MTX_PICKLE_KEY` as a secret and use it as an env-var:

```sh
env MTX_PICKLE_KEY='...' mtx # secret: lose it, lose the store
```

The store `$XDG_DATA_HOME/mtx/mtx.db` (`~/.local/share/mtx/mtx.db`)
holds E2EE keys and sync state. It is created with directory mode 0700 and
file mode 0600. mtx refuses to start if an existing store file or its WAL files are
not 0600. Keep the pickle key and the store out of git.

## What you get

- **Rooms as channels:** joined rooms are `&name` channels. `/create &name` makes an
  encrypted private room; `/join &name` joins by alias on your server.
- **Invites:** accepted automatically, with a toast. Declined invites and removals
  drop the channel.
- **History:** a room's earlier messages are fetched when you first see it. A resumed
  session does not replay what it already showed.
- **Threads:** a box above the chat lists thread heads and a `main thread` row. `T`
  starts a thread, `t` toggles between the main thread and the last thread, and sends
  inside a thread are thread replies.
- **Replies and edits:** replies and edits are real Matrix relations (`m.in_reply_to`,
  `m.replace`), not quoted text. Redactions and edits show in the scrollback.
- **Mentions:** mentions of you are highlighted and filterable (`m`).
- **Members:** the nick pane groups members by power level: admins, mods, members.
- **Status:** the chat frame title shows the channel's members, bypass and lines per
  minute. Typing shows as 💬 and unread counts show on channels and threads.
- **Encryption:** messages are encrypted on send and decrypted on receive. A message
  whose key has not arrived shows a placeholder and appears once the key comes in.
- **Links:** URLs and media download links in chat open with `$BROWSER`, `xdg-open` or
  `open`.

## Keys

| Key | Action |
|---|---|
| `i` | type a new message in the current thread or main chat |
| `a` / `A` | reply to my last or selected message (`A` quotes it in the text) |
| `C` | edit my last or selected message |
| `o` / `O` | reply in the last thread used (`O` quotes it in the text) |
| `T` | start a thread on the selected message, else the newest |
| `t` | toggle main thread / last thread |
| `I` | invite a user to this room (fuzzy pick) |
| `enter` | in INSERT: send, then back to NORMAL |
| `esc` | back to NORMAL; drops the reply, edit or quote |
| `tab` | focus a pane: chat, channels, nicks, threads |
| `ctrl+o` | fuzzy picker: channels, `@nicks` and threads |
| `m` | mentions only / all lines |
| `s` | show / hide times |
| `?` | help |

Press `?` for the full list.

## Commands

| Command | Action |
|---|---|
| `/join &room` | join a room by alias on your server |
| `/create &room` | create an encrypted private room |
| `/invite @user &room` | invite a user (a bare `@user` gets your server) |
| `/kick @user` | remove a member |
| `/part`, `/close` | leave the room; `/close` also forgets it |
| `/topic text` | set the room topic |
| `/names`, `/who` | list members |
| `/msg @user text` | send to the DM with that user, created if missing |
| `/whois @user` | show a user's profile |
| `/help`, `/quit` | help, quit |

## Files

- `cmd/mtx`: the TUI and the Matrix runner.
- `internal/matrix`: config, the SQLite store, the sync loop, E2EE and history.
- `internal/ui`, `internal/model`: the TUI, from ircc.
- `cmd/ircc`, `internal/peer`, `internal/registry`, `internal/rooms`, `internal/transcript`:
  the old Claude-session backend, kept until removed.

## Test

```sh
nix-shell -p go --run 'go test -tags goolm ./...'
```

Tests run against an in-memory mock homeserver (`maunium.net/go/mautrix/mockserver`).
Tests against a real homeserver are behind the `conduit` build tag and read
`MTX_CONDUIT_*` variables:

```sh
go test -tags "goolm conduit" -count=1 -run TestConduitX ./internal/matrix/
```

## License

See [LICENSE](LICENSE).
