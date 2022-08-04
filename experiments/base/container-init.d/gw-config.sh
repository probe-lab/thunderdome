#!/bin/sh
## The shell in the go-ipfs container is busybox, so a version of ash
## Shellcheck might warn on things POSIX sh cant do, but ash can
## In Shellcheck, ash is an alias for dash, but busybox ash can do more than dash 
## https://github.com/koalaman/shellcheck/blob/master/src/ShellCheck/Data.hs#L134
echo "setting Gateway config"
ipfs config --json Gateway '{
        "HTTPHeaders": {
            "Access-Control-Allow-Origin": [
                "*"
            ]
        },
        "RootRedirect": "",
        "Writable": false,
        "APICommands": [],
        "NoFetch": false,
        "NoDNSLink": false,
        "PublicGateways": {
            "dweb.link": {
                "NoDNSLink": false,
                "Paths": [
                    "/ipfs",
                    "/ipns",
                    "/api"
                ],
                "UseSubdomains": true
            },
            "gateway.ipfs.io": {
                "NoDNSLink": false,
                "Paths": [
                    "/ipfs",
                    "/ipns",
                    "/api"
                ],
                "UseSubdomains": false
            },
            "ipfs.io": {
                "NoDNSLink": false,
                "Paths": [
                    "/ipfs",
                    "/ipns",
                    "/api"
                ],
                "UseSubdomains": false
            }
        }
    }'

