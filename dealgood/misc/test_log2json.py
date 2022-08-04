import pytest

IPFSIO_LINE = """12.12.12.12 - admin [2022-08-03T00:00:26+00:00] "GET /ipfs/bafybeiabuqekzeraharmvsd3absedsxlwu3zxgn4vlhr74jf7nk63hshey/78.png HTTP/1.0" 206 84992 895 0.001 0.001 0.001 "-" "python-requests/2.27.1" _ backend http"""
DWEB_LINE = """11.11.11.11 - admin [2022-08-03T00:00:28+00:00] "GET /3291.png HTTP/1.0" 206 94129 288 0.001 0.001 0.001 "-" "curl/7.79.1" _ bafybeiglbxyoc3xcqee4l3vyq6f32nj2rrkbferekhmzcl462aiygekdry.ipfs.dweb.link http"""

from log2json import parse

def test_parse_ipfsio():
    request = parse(IPFSIO_LINE)
    assert request == {
        "method": "GET",
        "URI": "/ipfs/bafybeiabuqekzeraharmvsd3absedsxlwu3zxgn4vlhr74jf7nk63hshey/78.png",
        "header": {"Host":"ipfs.io" }
    }

def test_parse_dweb():
    request = parse(DWEB_LINE)
    assert request == {
        "method": "GET",
        "URI": "/3291.png",
        "header": {"Host":"bafybeiglbxyoc3xcqee4l3vyq6f32nj2rrkbferekhmzcl462aiygekdry.ipfs.dweb.link" }
    }
