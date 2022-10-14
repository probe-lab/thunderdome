#!/bin/bash
set -euo pipefail
# set -x
TAG=$(date +%F__%H%M)
docker build -t dealgood:"$TAG" .
docker login --username AWS 147263665150.dkr.ecr.eu-west-1.amazonaws.com -p $(aws ecr get-login-password --region eu-west-1)
docker tag dealgood:"$TAG" 147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:"$TAG"
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/dealgood:"$TAG"
echo "Tag was $TAG"