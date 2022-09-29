#!/bin/bash

## Checks out a specific commit from kubo repo and builds a docker image for it
## Image will be named 'thunderdome:kubo-commit-<hash>'

if [ $# -ne 1 ]; then
    echo "Expecting 1 arg"
    echo "./build.sh <COMMIT>"
    exit 1
fi

set -euo pipefail
# set -x

DOCKER_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
GIT_REPO="https://github.com/ipfs/kubo.git"
COMMIT=$1
KUBO_IMAGE_TAG=${COMMIT:0:9}
KUBO_IMAGE_NAME=thunderdome:kubo-commit-"$KUBO_IMAGE_TAG"
KUBO_IMAGE=$DOCKER_REPO/$KUBO_IMAGE_NAME

WORKDIR=$(mktemp -d)
if [ ! -e "$WORKDIR" ]; then
    echo >&2 "Failed to create temp directory"
    exit 1
fi

trap 'rm -rf "$WORKDIR"' EXIT

echo "Using work directory $WORKDIR"
cd "$WORKDIR"
git clone "$GIT_REPO" kubo

cd kubo
git checkout "$COMMIT"
docker build -t "$KUBO_IMAGE_NAME" .

aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin $DOCKER_REPO

docker tag "$KUBO_IMAGE_NAME" "$KUBO_IMAGE"
docker push "$KUBO_IMAGE"
