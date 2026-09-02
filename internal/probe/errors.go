package probe

import "errors"

// errNoSamples means every latency probe failed. Distinct from a
// transport error because the cause is usually a blocked endpoint
// rather than a broken link.
var errNoSamples = errors.New("no latency samples completed")

// ErrRateLimited means the endpoint refused the request with 429.
//
// It is called out separately because it says nothing about the link:
// the test asked for too much, too often. Repeated runs in quick
// succession are the usual cause.
var ErrRateLimited = errors.New("the speed test endpoint is rate limiting this client; wait a minute and try again")
