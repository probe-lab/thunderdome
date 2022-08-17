#!/bin/sh
## The shell in the go-ipfs container is busybox, so a version of ash
## Shellcheck might warn on things POSIX sh cant do, but ash can
## In Shellcheck, ash is an alias for dash, but busybox ash can do more than dash 
## https://github.com/koalaman/shellcheck/blob/master/src/ShellCheck/Data.hs#L134
echo "setting config"
ipfs config --json AutoNAT '{
      "ServiceMode": "enabled",
      "Throttle": {
        "GlobalLimit": 0,
        "Interval": "1m",
        "PeerLimit": 3
      }
    }'

ipfs config --json Datastore.BloomFilterSize '268435456'
ipfs config --json Datastore.StorageGCWatermark 90
ipfs config --json Datastore.StorageMax '"200GB"'
ipfs config --json Datastore.GCPeriod '"10m"'

ipfs config --json Pubsub.StrictSignatureVerification false

ipfs config --json Reprovider.Interval '"0"'

ipfs config --json Routing.Type '"dhtserver"'

ipfs config --json Swarm.ConnMgr.GracePeriod '"2m"'
ipfs config --json Swarm.ConnMgr.HighWater 5000
ipfs config --json Swarm.ConnMgr.LowWater 3000
ipfs config --json Swarm.ConnMgr.DisableBandwidthMetrics true

ipfs config --json Experimental.AcceleratedDHTClient true
ipfs config --json Experimental.StrategicProviding true
