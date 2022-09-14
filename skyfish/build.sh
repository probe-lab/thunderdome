#!/bin/bash
set -euo pipefail
# set -x
TAG=$(date +%F__%H%M)
docker build -t skyfish:"$TAG" .
docker login --username AWS 147263665150.dkr.ecr.eu-west-1.amazonaws.com -p $(aws ecr get-login-password --region eu-west-1)
docker tag skyfish:"$TAG" 147263665150.dkr.ecr.eu-west-1.amazonaws.com/skyfish:"$TAG"
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/skyfish:"$TAG"
echo "Tag was $TAG"
