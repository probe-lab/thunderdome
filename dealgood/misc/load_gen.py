import json
import sys
import random
import jsonlines

REQUESTS = [
    {
        "method": "GET",
        "URI": "/ipfs/QmQPeNsJPyVWPFDVHb77w8G42Fvo15z4bG2X8D2GhfbSXc/readme",
        "header": {"Host":"ipfs.io" },
    },
    {
        "method": "GET",
        "URI": "/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
        "header": {"Host":"ipfs.io" },
    },
    {
        "method": "GET",
        "URI": "/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
        "header": {"Accept": "application/vnd.ipld.car", "Host":"ipfs.io"},
    },
]

if __name__ == '__main__':
    for i in range(1000):
        request = random.choice(REQUESTS)
        with jsonlines.Writer(sys.stdout) as output:
            output.write(request)
