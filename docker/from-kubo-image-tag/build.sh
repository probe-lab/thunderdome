#!/bin/bash
# set -x

## Builds a version of kubo from a specific TAG and adds in
## gateway specific configuration
## Set REPO_NAME to override the default of ipfs/kubo
## Image will be named 'thunderdome:gw-TAG'


# https://hub.docker.com/r/ipfs/kubo/tags
if [ -z "$1" ]; then
  echo "Need to pass TAG, eg"
  echo "./build.sh v0.15.0"
  echo "You can also set REPO_NAME to override that, ipfs/kubo is the default"
  exit
fi
if [ -z "$REPO_NAME" ]; then
  REPO_NAME=ipfs/kubo
fi
set -euo pipefail
TAG=$1
IMAGE_NAME=thunderdome:gw-$TAG
ECR_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker build -t "$IMAGE_NAME" --build-arg TAG="$TAG" --build-arg REPO_NAME="$REPO_NAME" .
docker login -u AWS -p $(aws ecr get-login-password --region eu-west-1) "$ECR_REPO"
docker tag "$IMAGE_NAME" "$ECR_REPO"/"$IMAGE_NAME"
docker push "$ECR_REPO"/"$IMAGE_NAME"
echo "$ECR_REPO"/"$IMAGE_NAME"
