package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"
)

// metaLimit caps the meta response. The real one is well under a
// kilobyte; anything larger is a captive portal's login page, not the
// JSON we asked for.
const metaLimit = 16 << 10

// metaResponse is the subset of /meta this app reads. The endpoint
// returns more; unknown fields are dropped by the decoder, so it may
// grow without breaking us.
type metaResponse struct {
	ClientIP       string `json:"clientIp"`
	HTTPProtocol   string `json:"httpProtocol"`
	ASN            int    `json:"asn"`
	ASOrganization string `json:"asOrganization"`
	City           string `json:"city"`
	Region         string `json:"region"`
	Country        string `json:"country"`
	Colo           struct {
		IATA string  `json:"iata"`
		City string  `json:"city"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	} `json:"colo"`
}

// fetchMeta fills in what /cdn-cgi/trace does not carry: which network
// the client is on, and the datacenter's own idea of where it is.
//
// It writes into t rather than returning a second struct, because every
// field it adds answers the same question the trace already answers —
// who is at each end of this connection.
//
// The caller treats a failure as cosmetic: the trace has already
// succeeded by this point, so a run that loses this call is a run with
// a shorter connection panel, not a failed run.
func fetchMeta(ctx context.Context, cfg Config, t *Trace) error {
	if t == nil {
		return errors.New("meta: nil trace")
	}
	url := strings.TrimRight(cfg.BaseURL, "/") + "/meta"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)
	// The endpoint answers 403 without it. It is the page this app
	// stands in for, so the header is accurate as well as necessary.
	req.Header.Set("Referer", DefaultBaseURL+"/")

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("meta request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("meta: unexpected status %s", resp.Status)
	}

	// Cap the body: the real payload is <1k, anything larger is a
	// captive portal page. LimitReader alone would silently truncate
	// and a valid prefix could still decode, so read with limit+1 and
	// reject if over.
	lr := io.LimitReader(resp.Body, metaLimit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return fmt.Errorf("meta read: %w", err)
	}
	if len(data) > metaLimit {
		return fmt.Errorf("meta: response too large (%d bytes)", len(data))
	}
	var m metaResponse
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("meta decode: %w", err)
	}

	applyMeta(t, &m)
	return nil
}

// applyMeta copies the meta fields onto a trace, leaving anything the
// response did not carry as the trace already had it.
func applyMeta(t *Trace, m *metaResponse) {
	if t == nil || m == nil {
		return
	}
	// ASN is a 32-bit number. Clamp hostile values rather than
	// displaying a negative or overflowed number.
	if m.ASN < 0 || m.ASN > 4294967295 {
		t.ASN = 0
	} else {
		t.ASN = m.ASN
	}
	t.ASOrg = clip(m.ASOrganization, 96)
	t.City = clip(m.City, 64)
	t.Region = clip(m.Region, 64)
	t.HTTPProtocol = clip(m.HTTPProtocol, 16)
	if t.IP == "" {
		t.IP = clip(m.ClientIP, 64)
	}
	if t.Loc == "" {
		t.Loc = clip(strings.ToUpper(m.Country), 16)
	}

	// The datacenter's own coordinates beat the embedded table: the
	// table is a snapshot, and Cloudflare opens sites faster than this
	// app is rebuilt.
	if m.Colo.City != "" {
		t.ColoCity = clip(m.Colo.City, 64)
	}
	if m.Colo.Lat != 0 || m.Colo.Lon != 0 {
		// Guard against NaN/Inf and out-of-range coordinates from a
		// hostile endpoint.
		lat, lon := m.Colo.Lat, m.Colo.Lon
		if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
			return
		}
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return
		}
		t.ColoLat = lat
		t.ColoLng = lon
		t.ColoKnown = true
	}
}

// clip bounds a field from a response this app does not control, so a
// hostile or broken endpoint cannot hand the UI a megabyte of text.
// Truncation keeps valid UTF-8 by not splitting a multi-byte rune.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	// Walk runes and keep the byte offset of the last boundary
	// within max. Use utf8 to avoid allocating string(r).
	pos := 0
	for _, r := range s {
		sz := utf8.RuneLen(r)
		if sz < 0 {
			sz = 1
		}
		if pos+sz > max {
			break
		}
		pos += sz
	}
	// Re-slice to pos bytes — pos is always a rune boundary.
	return s[:pos]
}
