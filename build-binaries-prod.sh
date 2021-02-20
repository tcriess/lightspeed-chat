#!/usr/bin/env bash

set -e -u -o pipefail

readonly BINARIES=(cmd/lightspeed-chat cmd/lightspeed-chat-admin)

go mod vendor
# pkger available via
# go get -u github.com/markbates/pkger/cmd/pkger

for bin in "${BINARIES[@]}"; do
    echo "Building $bin"
    # pkger -o ${bin}
    (cd "./${bin}/" && go build .)
done
