#!/bin/bash
# set -x
if [ -z "$1" ]; then
  echo "Need to pass TAG, eg"
  echo "./build.sh v0.15.0"
  exit
fi
set -euo pipefail
SOURCE_TAG=$1
IMAGE_NAME=thunderdome:kubo-$SOURCE_TAG-reposize
ECR_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin "$ECR_REPO"
docker build -t "$IMAGE_NAME"  --build-arg TAG="$SOURCE_TAG" .
docker tag "$IMAGE_NAME" "$ECR_REPO"/"$IMAGE_NAME"
docker push "$ECR_REPO"/"$IMAGE_NAME"
echo "$ECR_REPO"/"$IMAGE_NAME"
