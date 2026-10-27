#!/usr/bin/env bash
# Regenerate every demo gif in docs/public/demo.
# Needs: conduit running (see the justfile `conduit` recipe) and MTX_SEED_REG_TOKEN set.
# Run from anywhere; the script goes to the repo root itself.
set -euo pipefail
cd "$(dirname "$0")/../.."

go build -tags goolm -o mtx ./cmd/mtx
docs/tapes/keys.sh
go run -tags "goolm seed" ./cmd/mtx/seed/ >/dev/null

rm -f docs/tapes/.demo/mtx.db*

for tape in docs/tapes/*.tape; do
    echo "vhs $tape"
    vhs $tape
done
