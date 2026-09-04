package probe

import "errors"

// errNoSamples means every latency probe failed. Distinct from a
// transport error because the cause is usually a blocked endpoint
// rather than a broken link.
var errNoSamples = errors.New("no latency samples completed")

// ErrRateLimited means the endpoint turned the request away rather
// than answering it.
//
// It is called out separately because it says nothing about the link:
// the test asked for too much, too often. Repeated runs in quick
// succession are the usual cause.
var ErrRateLimited = errors.New("the speed test endpoint is refusing requests from this client; wait a minute, or try another server")

// refused reports whether a status means "not now" rather than a
// result.
//
// 429 is the polite answer and 403 is what the rest give: several of
// the public LibreSpeed servers sit behind a firewall that starts
// returning 403 to every path once a client has asked for enough in a
// short window. Reading that as a permissions problem would send the
// user looking for a login that does not exist.
func refused(status int) bool {
	return status == 429 || status == 403
}
