#!/bin/bash
set -euo pipefail
# set -x
CHECKSUM=$(cat Dockerfile agent.yaml | sha256sum | cut -d ' ' -f 1)
TAG=target-${CHECKSUM:0:7}
docker build -t grafana-agent:"$TAG" .
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker tag grafana-agent:"$TAG" 147263665150.dkr.ecr.eu-west-1.amazonaws.com/grafana-agent:"$TAG"
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/grafana-agent:"$TAG"
echo "Tag was $TAG"