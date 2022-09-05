#!/bin/bash
set -euo pipefail

kubo_releases=(
    v0.15.0
    v0.14.0
)

for release in "${kubo_releases[@]}"; do
    ./build.sh "$release"
done

rm container_daemon container_init_run
wget -nc https://raw.githubusercontent.com/ipfs/kubo/master/bin/container_daemon -O container_daemon
wget -nc https://raw.githubusercontent.com/ipfs/kubo/master/bin/container_init_run -O container_init_run

go_ipfs_releases=(
    v0.13.1
    v0.12.2
    v0.11.0
    v0.10.0
    v0.9.1
    v0.8.0
)


export REPO_NAME=go-ipfs
for release in "${go_ipfs_releases[@]}"; do
    ./build.sh "$release"
done
