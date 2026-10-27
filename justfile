help:
    just --list

build:
    go build -tags goolm -o mtx ./cmd/mtx

test:
    go test -tags goolm ./...

fmt:
    go fmt ./...

run *args:
    go run -tags goolm ./cmd/mtx {{args}}

vet:
    go vet -tags goolm ./...

ci: vet test conduit

conduit:
    #!/usr/bin/env bash
    set -euo pipefail
    conduit_bin=$(command -v conduit)
    db=$(mktemp -d)
    CONDUIT_CONFIG=.github/conduit.toml CONDUIT_DATABASE_PATH="$db" "$conduit_bin" > conduit.log 2>&1 &
    pid=$!
    trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$db"' EXIT
    for i in $(seq 1 60); do
        timeout 2 curl -sf http://127.0.0.1:6167/_matrix/client/versions >/dev/null && break
        sleep 1
    done
    timeout 2 curl -sf http://127.0.0.1:6167/_matrix/client/versions >/dev/null
    export MTX_CONDUIT_HOMESERVER=http://127.0.0.1:6167
    export MTX_CONDUIT_REG_TOKEN=$(grep -E '^registration_token' .github/conduit.toml | sed -E 's/^[^"]*"([^"]*)".*/\1/')
    reg() {
        timeout 10 curl -s -X POST http://127.0.0.1:6167/_matrix/client/v3/register -H 'content-type: application/json' \
            -d "{\"username\":\"$1\",\"password\":\"$2\",\"initial_device_display_name\":\"ci\",\"auth\":{\"type\":\"m.login.registration_token\",\"token\":\"$MTX_CONDUIT_REG_TOKEN\"}}"
    }
    field() { echo "$1" | sed -E "s/.*\"$2\":\"([^\"]*)\".*/\1/"; }
    pa=$(head -c 24 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 20)
    pb=$(head -c 24 /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 20)
    a=$(reg "ci_a$RANDOM" "$pa"); b=$(reg "ci_b$RANDOM" "$pb")
    export MTX_CONDUIT_A_USER=$(field "$a" user_id) MTX_CONDUIT_A_PASSWORD=$pa
    export MTX_CONDUIT_B_USER=$(field "$b" user_id) MTX_CONDUIT_B_PASSWORD=$pb
    timeout 300 go test -tags "goolm conduit" -count=1 -timeout 180s -run TestConduit ./internal/matrix/


# Regenerate the keys page and every demo gif. Needs conduit running and MTX_SEED_REG_TOKEN set.
gifs:
    #!/usr/bin/env bash
    set -euo pipefail
    conduit_bin=$(command -v conduit)
    db=$(mktemp -d)
    CONDUIT_CONFIG=.github/conduit.toml CONDUIT_DATABASE_PATH="$db" "$conduit_bin" > conduit.log 2>&1 &
    pid=$!
    trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$db"' EXIT
    for i in $(seq 1 60); do
        timeout 2 curl -sf http://127.0.0.1:6167/_matrix/client/versions >/dev/null && break
        sleep 1
    done
    timeout 2 curl -sf http://127.0.0.1:6167/_matrix/client/versions >/dev/null
    export MTX_CONDUIT_HOMESERVER=http://127.0.0.1:6167
    export MTX_SEED_REG_TOKEN=$(grep -E '^registration_token' .github/conduit.toml | sed -E 's/^[^"]*"([^"]*)".*/\1/')
    docs/tapes/make.sh

# Regenerate only the keys page.
keys: build
    docs/tapes/keys.sh
