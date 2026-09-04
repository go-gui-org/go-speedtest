package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
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
	url := cfg.resolveBackend().pingURL(cfg)

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

	if refused(resp.StatusCode) {
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

// rttSampler times round trips in the background while a transfer runs.
//
// The interesting latency number is not the idle one. A link that
// answers in 20ms when nothing is happening and 900ms while a file is
// downloading has a full buffer somewhere in the path, and that is what
// makes calls stutter and pages hang mid-transfer. Measuring it needs
// samples taken during the load, not before it.
//
// The probe is a zero-byte request, so it costs a round trip and
// nothing else: it rides the queue it is measuring rather than adding
// to it.
type rttSampler struct {
	cancel context.CancelFunc
	done   chan []time.Duration
}

// startRTTSampler begins sampling until stop is called. Samples are
// emitted as they arrive, tagged with phase, so the chart fills in
// during the transfer rather than all at once at the end.
func startRTTSampler(
	ctx context.Context, cfg Config, phase Phase, emit func(Event),
) *rttSampler {
	ctx, cancel := context.WithCancel(ctx)
	s := &rttSampler{cancel: cancel, done: make(chan []time.Duration, 1)}

	go func() {
		var out []time.Duration
		start := time.Now()
		t := time.NewTicker(cfg.LoadedRTTInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				s.done <- out
				return
			case <-t.C:
			}
			// A sample that fails under load is not news: the transfer
			// is saturating the link and one probe lost the race. The
			// phase's own error handling covers a link that is
			// actually broken.
			rtt, err := measureRTT(ctx, cfg)
			if err != nil {
				continue
			}
			out = append(out, rtt)
			emit(Event{
				Kind:    EventRTT,
				Phase:   phase,
				Elapsed: time.Since(start),
				RTT:     rtt,
			})
		}
	}()
	return s
}

// stop ends sampling and returns everything collected. It waits for the
// goroutine to finish, so no event can arrive after it returns.
func (s *rttSampler) stop() []time.Duration {
	s.cancel()
	return <-s.done
}
