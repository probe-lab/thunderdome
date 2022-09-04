#!/bin/sh
## The shell in the go-ipfs container is busybox, so a version of ash
## Shellcheck might warn on things POSIX sh cant do, but ash can
## In Shellcheck, ash is an alias for dash, but busybox ash can do more than dash 
## https://github.com/koalaman/shellcheck/blob/master/src/ShellCheck/Data.hs#L134

if [ -z "$REPO_SIZE" ]; then
  echo "Need to set REPO_SIZE, eg to 200GB"
  exit 1
fi
echo "setting StorageMax to $REPO_SIZE"
ipfs config Datastore.StorageMax $REPO_SIZE
