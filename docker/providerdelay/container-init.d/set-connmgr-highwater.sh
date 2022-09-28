if [ -z "$CONNMGR_HIGHWATER" ]; then
	echo "using default Swarm.ConnMgr.HighWater value"
  exit 0
fi
echo "setting Swarm.ConnMgr.HighWater to $CONNMGR_HIGHWATER"
ipfs config --json Swarm.ConnMgr.HighWater $CONNMGR_HIGHWATER
