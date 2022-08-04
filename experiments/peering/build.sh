#!/bin/bash
set -euo pipefail
# set -x

docker build -t thunderdome:peering-with with/
docker build -t thunderdome:peering-without without/
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 147263665150.dkr.ecr.eu-west-1.amazonaws.com
docker tag thunderdome:peering-with 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-with
docker tag thunderdome:peering-without 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-without
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-with
docker push 147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome:peering-without 
