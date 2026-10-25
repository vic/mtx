build:
    nix-shell -p go --run 'go build -tags goolm -o mtx ./cmd/mtx'

test:
    nix-shell -p go --run 'go test -tags goolm ./...'

fmt:
    nix-shell -p go --run 'go fmt ./...'

login:
    nix-shell -p go --run 'go run -tags goolm ./cmd/mtx login'

run:
    nix-shell -p go --run 'go run -tags goolm ./cmd/mtx'

vet:
    nix-shell -p go --run 'go vet -tags goolm ./...'

ci: vet test conduit

conduit:
    #!/usr/bin/env bash
    set -euo pipefail
    conduit_bin=$(nix-shell -p matrix-conduit --run 'command -v conduit')
    CONDUIT_CONFIG=.github/conduit.toml "$conduit_bin" > conduit.log 2>&1 &
    pid=$!
    trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT
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
    timeout 300 nix-shell -p go --run 'go test -tags "goolm conduit" -count=1 -timeout 180s -run TestConduit ./internal/matrix/'

