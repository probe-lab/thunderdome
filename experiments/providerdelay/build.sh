#!/bin/bash
set -euo pipefail
# set -x

EXP_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
CONFIGS="$EXP_DIR/image/configs/*.sh"
DOCKER_REPO=147263665150.dkr.ecr.eu-west-1.amazonaws.com
KUBO_IMAGE_NAME=thunderdome:providerdelay-kubo
KUBO_IMAGE=$DOCKER_REPO/$KUBO_IMAGE_NAME

WORKDIR=$(mktemp -d)
if [ ! -e "$WORKDIR" ]; then
    >&2 echo "Failed to create temp directory"
    exit 1
fi

trap 'rm -rf "$WORKDIR"' EXIT

echo "Using work directory $WORKDIR"
cd "$WORKDIR"
git clone https://github.com/iand/go-ipfs.git kubo

cd kubo
git switch feat/remove-provider-search-delay
docker build -t $KUBO_IMAGE_NAME .

aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin $DOCKER_REPO

docker tag $KUBO_IMAGE_NAME $KUBO_IMAGE
docker push $KUBO_IMAGE


cd "$EXP_DIR"

for cf in $CONFIGS
do
	if [ -f "$cf" ]
 	then
 		file=${cf##*/}
 		name=${file%.sh}
 		# Build and push image
 		echo Building thunderdome:$name
		docker build --build-arg config=$file --build-arg kuboimage=$KUBO_IMAGE -t thunderdome:$name image/
		docker tag thunderdome:$name $DOCKER_REPO/thunderdome:$name
		docker push $DOCKER_REPO/thunderdome:$name
	else
    	>&2 echo "\"$cf\" was not a file"
    	exit 1
  	fi

done
