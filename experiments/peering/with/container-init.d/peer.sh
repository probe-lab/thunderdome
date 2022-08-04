#!/bin/sh

echo "setting Peers"
PEERS=$(cat /container-init.d/peers.json)
ipfs config --json Peering.Peers "$PEERS"