#!/usr/bin/env bash
# Run the mtx TUI against the local conduit. Each launch registers a fresh disposable user and
# uses a fresh store, so nothing (token, pickle key) is kept after exit.
# Usage: /tmp/mtx.sh   (run once per terminal; two terminals = two users to chat with)
set -euo pipefail

REPO=/home/vic/hk/mtx
HS=http://127.0.0.1:6167
BIN=/tmp/mtx-tui-bin
TOK=$(sed -n 's/^registration_token = "\(.*\)"/\1/p' "$HOME/conduit.toml")
[ -n "$TOK" ] || { echo "no registration_token in ~/conduit.toml" >&2; exit 1; }

echo "mtx: building (first run takes a moment)..." >&2
(cd "$REPO" && CGO_ENABLED=0 nix-shell -p go --run "go build -tags goolm -o $BIN ./cmd/mtx")

NAME="mtxtui$RANDOM$RANDOM"
PW=$(head -c 24 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 20)
JSON=$(curl -sf -X POST "$HS/_matrix/client/v3/register" -H 'content-type: application/json' \
  -d "{\"username\":\"$NAME\",\"password\":\"$PW\",\"initial_device_display_name\":\"mtx-tui\",\"auth\":{\"type\":\"m.login.registration_token\",\"token\":\"$TOK\"}}")
field() { printf '%s' "$JSON" | sed -E "s/.*\"$1\":\"([^\"]*)\".*/\1/"; }

export MTX_HOMESERVER="$HS"
export MTX_USER_ID="$(field user_id)"
export MTX_ACCESS_TOKEN="$(field access_token)"
export MTX_DEVICE_ID="$(field device_id)"
export MTX_PICKLE_KEY="$(head -c 32 /dev/urandom | base64 -w0)"
export MTX_DB="$(mktemp -d /tmp/mtx-tui.XXXXXX)/mtx.db"

echo "mtx: $MTX_USER_ID (store $MTX_DB)" >&2
read
exec "$BIN"
