package filter

import (
	"strings"

	ipfspath "github.com/ipfs/go-path"
	"github.com/probe-lab/thunderdome/pkg/request"
)

// A RequestFilter reports whether a request meets a condition
type RequestFilter func(*request.Request) bool

// A NullRequestFilter allows every request to pass
func NullRequestFilter(*request.Request) bool {
	return true
}

// A PathRequestFilter only allows path requests to pass
func PathRequestFilter(req *request.Request) bool {
	if req.Method != "GET" {
		return false
	}
	if !strings.HasPrefix(req.URI, "/ipfs") && !strings.HasPrefix(req.URI, "/ipns") {
		return false
	}
	return true
}

// A ValidPathRequestFilter only allows valid path requests to pass
func ValidPathRequestFilter(req *request.Request) bool {
	if !PathRequestFilter(req) {
		return false
	}
	path := req.URI
	if p := strings.Index(path, "?"); p != -1 {
		path = path[:p]
	}
	if p := strings.Index(path, "#"); p != -1 {
		path = path[:p]
	}
	if _, err := ipfspath.ParsePath(path); err != nil {
		return false
	}

	return true
}
