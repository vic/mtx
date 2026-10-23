build:
    nix-shell -p go --run 'go build -tags goolm -o mtx ./cmd/mtx'

test:
    nix-shell -p go --run 'go test -tags goolm ./...'

fmt:
    nix-shell -p go --run 'go fmt ./...'

run:
    nix-shell -p go --run 'go run -tags goolm ./cmd/mtx'

vet:
    nix-shell -p go --run 'go vet -tags goolm ./...'
