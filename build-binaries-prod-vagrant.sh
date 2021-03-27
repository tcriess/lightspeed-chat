#!/usr/bin/env bash

set -e -u -o pipefail

readonly BINARIES=(cmd/lightspeed-chat cmd/lightspeed-chat-admin plugins/lightspeed-chat-base-commands-plugin plugins/lightspeed-chat-google-translate-plugin)

go mod vendor

for bin in "${BINARIES[@]}"; do
    echo "Building $bin"
    vagrant docker-exec -- bash -c "cd /vagrant/${bin}/ && go build ."
done
