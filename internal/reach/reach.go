// Package reach is how requiem talks to model endpoints that may be out of
// reach: an HTTP client that stops trying to connect after a few seconds, and
// a test for the failures that mean the server could not be reached at all.
// A LAN server seen from a laptop elsewhere does not refuse connections, it
// drops them, so without a connect bound every call waited out the whole
// request timeout.
// requiem: embedding/fail-fast-unreachable
package reach

import (
	"errors"
	"net"
	"net/http"
	"time"
)

// ConnectTimeout bounds connecting, separately from the request timeout: a
// server that accepts the connection and answers slowly still gets the full
// request timeout, since a large batch or a cold model legitimately takes it.
const ConnectTimeout = 2 * time.Second

// DownFor is how long an endpoint that failed to connect is skipped by
// commands that can do without it.
const DownFor = 5 * time.Minute

// Client returns an HTTP client whose requests time out after timeout and
// whose connections give up after ConnectTimeout.
func Client(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 2 * ConnectTimeout
	return &http.Client{Timeout: timeout, Transport: tr}
}

// Unreachable reports whether err means the server could not be reached at
// all: the connection was refused or timed out, or the name did not resolve.
// A slow answer or an error the server returned is not unreachability.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	var dns *net.DNSError
	return errors.As(err, &dns)
}
