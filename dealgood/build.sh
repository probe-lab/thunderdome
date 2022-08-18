#!/bin/bash
set -euo pipefail
# set -x
TAG=latest
docker build -t dealgood:"$TAG" .
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker tag dealgood:"$TAG" 147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:"$TAG"
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:"$TAG"
echo "Tag was $TAG"