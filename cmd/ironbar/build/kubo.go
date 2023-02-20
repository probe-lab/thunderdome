package build

var BaseConfigs = map[string][]string{
	"kubo-default": {},
	"bifrost": {
		`ipfs config --json AutoNAT '{"ServiceMode": "disabled"}'`,
		`ipfs config --json Datastore.BloomFilterSize '268435456'`,
		`ipfs config --json Datastore.StorageGCWatermark 90`,
		`ipfs config --json Datastore.StorageMax '"160GB"'`,
		`ipfs config --json Pubsub.StrictSignatureVerification false`,
		`ipfs config --json Reprovider.Interval '"0"'`,
		`ipfs config --json Routing.Type '"dhtserver"'`,
		`ipfs config --json Swarm.ConnMgr.GracePeriod '"2m"'`,
		`ipfs config --json Swarm.ConnMgr.HighWater 5000`,
		`ipfs config --json Swarm.ConnMgr.LowWater 3000`,
		`ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true`,
		`ipfs config --json Experimental.AcceleratedDHTClient true`,
		`ipfs config --json Experimental.StrategicProviding true`,
	},
}
