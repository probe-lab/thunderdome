package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

type Request struct {
	Method string            `json:"method"`
	URI    string            `json:"uri"`
	Body   []byte            `json:"body,omitempty"`
	Header map[string]string `json:"header"`
}

// A RequestSource is a provider of a stream of requests that can be sent to workers.
type RequestSource interface {
	// Next advances to the next requests. It returns false if no more requests are
	// available or a fatal error occured while advancing the stream.
	Next() bool

	// Request returns the current request from the stream.
	Request() Request

	// Err returns any error that was encountered while advancing the stream.
	Err() error
}

// StdinRequestSource is a request source that reads a stream of JSON requests
// from stdin.
type StdinRequestSource struct {
	scanner *bufio.Scanner
	req     Request
	err     error
}

var _ RequestSource = (*StdinRequestSource)(nil)

func NewStdinRequestSource() *StdinRequestSource {
	return &StdinRequestSource{
		scanner: bufio.NewScanner(os.Stdin),
	}
}

func (s *StdinRequestSource) Next() bool {
	if s.err != nil {
		return false
	}

	if !s.scanner.Scan() {
		s.err = s.scanner.Err()
		return false
	}

	data := s.scanner.Bytes()
	s.req = Request{}
	s.err = json.Unmarshal(data, &s.req)
	return s.err == nil
}

func (s *StdinRequestSource) Request() Request {
	return s.req
}

func (s *StdinRequestSource) Err() error {
	return s.err
}

// RandomRequestSource is a request source that provides a random request
// from a list of requests.
type RandomRequestSource struct {
	reqs []*Request
	idx  int
	rng  *rand.Rand
}

func NewRandomRequestSource(reqs []*Request) *RandomRequestSource {
	return &RandomRequestSource{
		reqs: reqs,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *RandomRequestSource) Next() bool {
	r.idx = r.rng.Intn(len(r.reqs))
	return true
}

func (r *RandomRequestSource) Request() Request {
	return *r.reqs[r.idx]
}

func (r *RandomRequestSource) Err() error {
	return nil
}

// NewNginxLogRequestSource reads a stream of requests
// from an nginx formatted access log file and returns a RandomRequestSource
// that will serve the requests at random. Requests are filtered to GET
// and paths /ipfs and /ipns
func NewNginxLogRequestSource(fname string) (*RandomRequestSource, error) {
	var reqs []*Request

	f, err := os.Open(fname)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		pos1 := bytes.IndexRune(line, '"')
		pos2 := bytes.IndexRune(line[pos1+1:], '"')

		fields := strings.SplitN(string(line[pos1+1:pos1+pos2+1]), " ", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[0] != "GET" {
			continue
		}
		if !strings.HasPrefix(fields[1], "/ipfs") && !strings.HasPrefix(fields[1], "/ipns") {
			continue
		}

		reqs = append(reqs, &Request{
			Method: fields[0],
			URI:    fields[1],
		})
	}

	if scanner.Err() != nil {
		return nil, scanner.Err()
	}

	return NewRandomRequestSource(reqs), nil
}

var samplePathsIPFS = []string{
	"/ipfs/QmQPeNsJPyVWPFDVHb77w8G42Fvo15z4bG2X8D2GhfbSXc/readme",
	"/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
	"/ipfs/QmUa7f9JtJMsqJJ3s3ZXk6WyF4xJLE8FiqYskZGgk8GCDv",
	"/ipfs/QmaQsTLL3nc5dw6wAvaioJSBfd1jhQrA2o6ucFf7XeV74P",
	"/ipfs/QmWS73SiuSK1zZ2nVSmUb2xZfSwHcdgrYkmXn2ELpQ5XRT",
	"/ipfs/QmVxjFRyhmyQaZEtCh7nk2abc7LhFkzhnRX4rcHqCCpikR",
	"/ipfs/QmUiRx71uxfmUE8V3H9sWAsAXoM88KR4eo1ByvvcFNeTLR",
	"/ipfs/QmcS5JZs8X3TdtkEBpHAdUYjdNDqcL7fWQFtQz69mpnu2X",
	"/ipfs/QmfA31fbCWojSmhSGvvfxmxaYCpMoXP95zEQ9sLvBGHNaN",
	"/ipfs/QmR9i9KL3vhhAqTBGj1bPPC7LvkptxrH9RvxJxLN1vvsBE",
	"/ipfs/QmWV8rqZLxs1oQN9jxNWmnT1YdgLwCcscv94VARrhHf1T7",
	"/ipfs/QmamahpFCstMUqHi2qGtVoDnRrsXhid86qsfvoyCTKJqHr",
	"/ipfs/QmWionkqH2B6TXivzBSQeSyBxojaiAFbzhjtwYRrfwd8nH",
	"/ipfs/Qmf93EMrADXAK6CyiSfE8xx45fkMfR3uzKEPCvZC1n2kzb",
	"/ipfs/QmWS73SiuSK1zZ2nVSmUb2xZfSwHcdgrYkmXn2ELpQ5XRT",
	"/ipfs/QmNT23NWCVFFw9ioBjUCMcBXpHTDgr7tKzaj1ckm5UPWT1/ipfs-029.w3c-blockchain-workshop.compressed.pdf",
	"/ipfs/QmR7tiySn6vFHcEjBeZNtYGAFh735PJHfEMdVEycj9jAPy/docs/getting-started",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM/rfc-data/rfc1113.txt",
	"/ipfs/QmNvTjdqEPjZVWCvRWsFJA1vK7TTw1g9JP6we1WBJTRADM/rfc-data/rfc1147.pdf",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/pages",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/thumbnails/21027771304_43d7ae4edc_o.jpg._t.jpg",
	"/ipfs/QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D/frontend/pages/QXBvbGxvIDE1IE1hZ2F6aW5lIDkzL1A=.html",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/food/aphrodis.txt",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/food/ppbeer.txt",
	"/ipfs/QmNoscE3kNc83dM5rZNUC5UDXChiTdDcgf16RVtFCRWYuU/humor/aclamt.txt",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/SUBSITES/ar.geocities.com.7z.009",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/SUBSITES/de.geocities.com.7z.066",
	"/ipfs/QmVCjhoEFC9vwvaa8bKyJgwAByP4MXSogcyDGoz4Lkc3ox/GEOCITIES/www.geocities.com.7z.011",
}

var samplePathsIPNS = []string{
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.meta",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.params",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0170db1f394b35d995252228ee359194b13199d259380541dc529fb0099096b0.vk",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.meta",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.params",
	"/ipns/proofs.filecoin.io/v28-proof-of-spacetime-fallback-merkletree-poseidon_hasher-8-0-0-0cfb4f178bbb71cf2ecfcd42accce558b27199ab4fb59cb78f2483fe21ef36d9.vk",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/United_Kingdom",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Rugby_School",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/John_Locke",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Vertigo_(film)",
	"/ipns/en.wikipedia-on-ipfs.org/wiki/Fleetwood_Mac",
	"/ipns/QmYoQ4Gn9vAcimaXT5xWYAPrBCu3QZyLmEvhLFu9djNZCy/whitelist.txt",
	"/ipns/ipfs-planets.echox.app/mainnet/GQJqkw49LrbLAKa/480/echox-nft.gif",
	"/ipns/ipfs-planets.echox.app/mainnet/GQJqkw49LrbLAKa/66/metadata.json",
	"/ipns/fromthemachine.org/ARTIMESIAN.html",
}

func sampleRequests() []*Request {
	paths := []string{}
	paths = append(paths, samplePathsIPFS...)
	paths = append(paths, samplePathsIPNS...)
	return permutePaths(paths)
}

func permuteSamplePathsIPFS() []*Request {
	return permutePaths(samplePathsIPFS)
}

func permuteSamplePathsIPNS() []*Request {
	return permutePaths(samplePathsIPNS)
}

func permutePaths(paths []string) []*Request {
	headerVariants := []map[string]string{
		{},
		{"Accept": "application/vnd.ipld.car"},
		{"Accept": "application/vnd.ipld.raw"},
	}

	reqs := make([]*Request, 0, len(paths)*len(headerVariants))
	for _, p := range paths {
		for _, h := range headerVariants {
			req := &Request{
				Method: "GET",
				URI:    p,
				Header: map[string]string{},
			}

			for k, v := range h {
				req.Header[k] = v
			}

			reqs = append(reqs, req)
		}
	}

	return reqs
}
