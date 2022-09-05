#!/bin/bash
# set -x
if [ -z "$1" ]; then
  echo "Need to pass TAG, eg"
  echo "./build.sh v0.15.0"
  exit
fi
if [ -z "$REPO_NAME" ]; then
  REPO_NAME=kubo
fi
set -euo pipefail
TAG=$1
IMAGE_NAME=thunderdome:kubo-$TAG
ECR_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker build -t "$IMAGE_NAME" --build-arg TAG="$TAG" --build-arg REPO_NAME="$REPO_NAME" .
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin "$ECR_REPO"
docker tag "$IMAGE_NAME" "$ECR_REPO"/"$IMAGE_NAME"
docker push "$ECR_REPO"/"$IMAGE_NAME"
echo "$ECR_REPO"/"$IMAGE_NAME"
