if [ -z "$CONNMGR_LOWWATER" ]; then
	echo "using default Swarm.ConnMgr.LowWater value"
  exit 0
fi
echo "setting Swarm.ConnMgr.LowWater to $CONNMGR_LOWWATER"
ipfs config --json Swarm.ConnMgr.LowWater $CONNMGR_LOWWATER
