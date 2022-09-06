#!/bin/bash
if [ $# -ne 3 ]; then
    echo "Expecting 3 args"
    echo "./build.sh <REPO> <BRANCH> <TAG_NAME>"
    exit 1
fi

set -euo pipefail
# set -x

DOCKER_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
GIT_REPO=$1
BRANCH_NAME=$2
KUBO_IMAGE_TAG=$3
KUBO_IMAGE_NAME=thunderdome:"$KUBO_IMAGE_TAG"
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
git switch "$BRANCH_NAME"
docker build -t "$KUBO_IMAGE_NAME" .

aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin $DOCKER_REPO

docker tag "$KUBO_IMAGE_NAME" "$KUBO_IMAGE"
docker push "$KUBO_IMAGE"
