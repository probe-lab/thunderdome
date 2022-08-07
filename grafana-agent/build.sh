#!/bin/bash
set -euo pipefail
# set -x

docker build -t grafana-agent:latest .
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker tag grafana-agent:latest 147263665150.dkr.ecr.eu-west-1.amazonaws.com/grafana-agent:latest
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/grafana-agent:latest
