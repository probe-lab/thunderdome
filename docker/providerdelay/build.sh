#!/bin/bash
# Build from git repo
set -euo pipefail
TAG_NAME=providerdelay
../from-git-repo/build.sh https://github.com/iand/go-ipfs.git feat/remove-provider-search-delay $TAG_NAME

# Add thunderdome config
cd ../from-kubo-image-tag
export REPO_NAME=147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome
./build.sh $TAG_NAME
cd -
## (creates a kubo-"$TAG_NAME" image)

# Build in this dir to add additional container-init.d
SOURCE_TAG=kubo-"$TAG_NAME"
FINAL_TAG=kubo-providerdelay-from-env
docker build -t "$FINAL_TAG" --build-arg SOURCE_TAG="$SOURCE_TAG" .
FINAL_REMOTE_TAG="$REPO_NAME":"$FINAL_TAG"
docker tag "$FINAL_TAG" "$FINAL_REMOTE_TAG"
docker push "$FINAL_REMOTE_TAG"
echo "$FINAL_REMOTE_TAG"
