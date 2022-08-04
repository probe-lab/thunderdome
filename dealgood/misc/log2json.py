import sys
import random
import jsonlines
import re
import os

#   log_format netdata '$remote_addr - $remote_user [$time_iso8601] '
#                      '"$request" $status $body_bytes_sent '
#                      '$request_length $request_time $upstream_response_time $upstream_header_time '
#                      '"$http_referer" "$http_user_agent" $server_name $http_host $scheme';

def parse(line: str):
    r = """.*?\"(?P<method>\w+) (?P<URI>[^\ ]+) [\w\/\.]+\" (?P<return_code>\d+) \d+ \d+ [\d\.\-]+ [\d\.\-]+ [\d\.\-]+ \"[^\"]+\" \"[^\"]+\" [^\ ]+ (?P<host_header>[^\ ]+) [^\ ]+$"""
    match = re.search(r, line)
    if not match:
        return None # This happened about 30 times in a days log and the requests were pretty mangled
    host = match['host_header']
    if host == 'backend':
        host = 'ipfs.io'
    return {
        "method": match["method"],
        "URI": match["URI"],
        "header": {"Host": host}
    }

if __name__ == '__main__':
    for line in sys.stdin:
        request = parse(line)
        if not request:
            continue
        playback_fraction = float(os.environ.get("PLAYBACK_FRACTION", "1"))
        with jsonlines.Writer(sys.stdout) as output:
            if random.random() < playback_fraction:
                output.write(request)
