package probe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// traceLimit caps the trace response. The real one is a few hundred
// bytes; anything larger is a redirect to a captive portal, not data we
// want to parse.
const traceLimit = 8 << 10

// fetchTrace asks the edge which datacenter answered and roughly where
// the client is.
//
// The response is a plain-text key=value list, one pair per line:
//
//	fl=123abc
//	ip=203.0.113.7
//	colo=SEA
//	loc=US
//
// Unknown keys are ignored, so the parser survives the endpoint adding
// fields.
func fetchTrace(ctx context.Context, cfg Config) (*Trace, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/cdn-cgi/trace"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trace request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trace: unexpected status %s", resp.Status)
	}

	t := &Trace{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, traceLimit))
	// A single line from a captive portal can otherwise push the
	// scanner's internal buffer to its growth limit.
	sc.Buffer(make([]byte, 0, 4096), 4096)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "colo":
			t.Colo = strings.ToUpper(strings.TrimSpace(val))
			if len(t.Colo) > 16 {
				t.Colo = t.Colo[:16]
			}
		case "ip":
			t.IP = strings.TrimSpace(val)
			if len(t.IP) > 64 {
				t.IP = t.IP[:64]
			}
		case "loc":
			t.Loc = strings.ToUpper(strings.TrimSpace(val))
			if len(t.Loc) > 16 {
				t.Loc = t.Loc[:16]
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("trace read: %w", err)
	}

	resolveLocations(t)
	return t, nil
}
