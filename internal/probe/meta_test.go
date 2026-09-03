package probe

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchMetaFillsTrace(t *testing.T) {
	e := newFakeEdge(t)
	cfg := New(Config{BaseURL: e.URL}).cfg

	tr := &Trace{Colo: "SEA"}
	if err := fetchMeta(context.Background(), cfg, tr); err != nil {
		t.Fatalf("fetchMeta: %v", err)
	}

	if tr.ASN != 64512 || tr.ASOrg != "EXAMPLE NET" {
		t.Errorf("network = %d %q", tr.ASN, tr.ASOrg)
	}
	if tr.City != "Peosta" || tr.Region != "Iowa" {
		t.Errorf("client location = %q %q", tr.City, tr.Region)
	}
	if tr.HTTPProtocol != "HTTP/2" {
		t.Errorf("protocol = %q", tr.HTTPProtocol)
	}
	// The datacenter's own coordinates must win over the embedded
	// table, which is what makes an unlisted colo still land on the map.
	if !tr.ColoKnown || tr.ColoCity != "Seattle" || tr.ColoLat != 47.45 {
		t.Errorf("colo = %+v", tr)
	}
}

func TestFetchMetaKeepsTraceValues(t *testing.T) {
	// The trace runs first, so anything it already answered stays as it
	// answered it: two sources disagreeing on the client's own address
	// would show as a flickering panel.
	tr := &Trace{IP: "198.51.100.9", Loc: "GB"}
	applyMeta(tr, &metaResponse{ClientIP: "203.0.113.7", Country: "us"})

	if tr.IP != "198.51.100.9" || tr.Loc != "GB" {
		t.Errorf("meta overwrote trace: %q %q", tr.IP, tr.Loc)
	}
}

func TestFetchMetaRejectsBadResponses(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"status": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
		"not json": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html>captive portal</html>"))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(h)
			defer srv.Close()
			cfg := New(Config{BaseURL: srv.URL}).cfg
			if err := fetchMeta(context.Background(), cfg, &Trace{}); err == nil {
				t.Error("no error")
			}
		})
	}
}

func TestClipBoundsHostileFields(t *testing.T) {
	long := strings.Repeat("x", 200)
	tr := &Trace{}
	applyMeta(tr, &metaResponse{ASOrganization: long, City: long})
	if len(tr.ASOrg) != 96 || len(tr.City) != 64 {
		t.Errorf("unclipped: org %d, city %d", len(tr.ASOrg), len(tr.City))
	}
	if got := clip("  spaced  ", 64); got != "spaced" {
		t.Errorf("clip did not trim: %q", got)
	}
}

func TestClipUTF8Safe(t *testing.T) {
	// "é" is 2 bytes; clipping at 3 bytes must not split it.
	s := "éééé" // 8 bytes
	if got := clip(s, 3); got != "é" {
		t.Errorf("clip UTF-8 = %q len %d, want é", got, len(got))
	}
	// Verify result is valid UTF-8.
	for _, r := range clip(s, 3) {
		if r == 0xFFFD {
			t.Error("clip produced invalid UTF-8")
		}
	}
}

func TestFetchMetaRejectsOversized(t *testing.T) {
	big := strings.Repeat("x", 17<<10) // > metaLimit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	cfg := New(Config{BaseURL: srv.URL}).cfg
	if err := fetchMeta(context.Background(), cfg, &Trace{}); err == nil {
		t.Error("oversized response: no error")
	}
}

func TestApplyMetaHardened(t *testing.T) {
	// Negative ASN must be clamped, not displayed.
	tr := &Trace{}
	applyMeta(tr, &metaResponse{ASN: -5})
	if tr.ASN != 0 {
		t.Errorf("ASN -5 = %d, want 0", tr.ASN)
	}
	// Invalid coordinates must not mark ColoKnown.
	tr = &Trace{}
	applyMeta(tr, &metaResponse{Colo: struct {
		IATA string  `json:"iata"`
		City string  `json:"city"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	}{Lat: 200, Lon: 0}})
	if tr.ColoKnown {
		t.Error("out-of-range lat marked known")
	}
	// NaN coordinates must not panic or mark known.
	tr = &Trace{}
	applyMeta(tr, &metaResponse{Colo: struct {
		IATA string  `json:"iata"`
		City string  `json:"city"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	}{Lat: math.NaN(), Lon: 0}})
	if tr.ColoKnown {
		t.Error("NaN lat marked known")
	}
	// Nil args must not panic.
	applyMeta(nil, nil)
	if err := fetchMeta(context.Background(), New(Config{BaseURL: "http://example.com"}).cfg, nil); err == nil {
		t.Error("nil trace: no error")
	}
}
