package main

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"os"
	"time"
)

type Request struct {
	Method string            `json:"method"`
	URI    string            `json:"uri"`
	Body   []byte            `json:"body,omitempty"`
	Header map[string]string `json:"header"`
}

var sampleRequests = []*Request{
	{
		Method: "GET",
		URI:    "/ipfs/QmQPeNsJPyVWPFDVHb77w8G42Fvo15z4bG2X8D2GhfbSXc/readme",
	},
	{
		Method: "GET",
		URI:    "/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
	},
	{
		Method: "GET",
		URI:    "/ipfs/bafkreifjjcie6lypi6ny7amxnfftagclbuxndqonfipmb64f2km2devei4",
		Header: map[string]string{"Accept": "application/vnd.ipld.car"},
	},
}

// A RequestSource is a provider of a stream of requests that can be sent to workers.
type RequestSource interface {
	// Next advances to the next requests. It returns false if no more requests are
	// available or a fatal error occured while advancing the stream.
	Next() bool

	// Request returns the current request from the stream.
	Request() *Request

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
	if s.err != nil {
		return false
	}
	return true
}

func (s *StdinRequestSource) Request() *Request {
	return &s.req
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

func (r *RandomRequestSource) Request() *Request {
	return r.reqs[r.idx]
}

func (r *RandomRequestSource) Err() error {
	return nil
}
