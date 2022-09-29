#!/bin/bash

if [ $# -ne 1 ]; then
    echo "Expecting 1 arg"
    echo "./build.sh <COMMIT>"
    exit 1
fi

set -euo pipefail
# set -x

# Build from commit in kubo's repo
COMMIT=$1
../from-kubo-commit/build.sh $COMMIT

# Add gateway config
cd ../from-kubo-image-tag
export REPO_NAME=147263665150.dkr.ecr.eu-west-1.amazonaws.com/thunderdome
TAG_NAME=kubo-commit-${COMMIT:0:9}
./build.sh $TAG_NAME
cd -
