package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// measureRTT times one round trip to the edge.
//
// The reading is time-to-first-byte on a zero-byte download, not a full
// request duration: TTFB isolates the network round trip from the time
// spent draining a body. It still includes the edge's own processing,
// so treat these numbers as "latency to a working HTTP response", which
// is what a user actually feels, rather than as an ICMP ping.
func measureRTT(ctx context.Context, cfg Config) (time.Duration, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/__down?bytes=0"

	var start, firstByte time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}
	req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(ctx, trace), http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	start = time.Now()
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("latency request: %w", err)
	}
	// Drain and close so the connection returns to the idle pool. Reusing
	// it is the point: a fresh TLS handshake on every sample would
	// measure setup cost, not latency.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("latency: unexpected status %s", resp.Status)
	}
	if firstByte.IsZero() {
		// No trace callback fired, which happens with some proxies.
		// Fall back to the whole-request time.
		return time.Since(start), nil
	}
	return firstByte.Sub(start), nil
}
