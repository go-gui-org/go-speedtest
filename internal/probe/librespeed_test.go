package probe

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// fakeLibre stands in for a LibreSpeed backend. It answers the three
// endpoints the engine uses plus getIP, and records what it was asked
// for, so a test can assert the request shape rather than a timing.
//
// The paths are nested under /backend on purpose: half the published
// server list is shaped that way, and it is the half most likely to
// break if the URL join loses or doubles a slash.
type fakeLibre struct {
	*httptest.Server

	// isp is the body getIP returns. Set per test to cover the three
	// answers the real servers give.
	isp string

	pingCalls atomic.Int64
	downCalls atomic.Int64
	upCalls   atomic.Int64
	downBytes atomic.Int64
	upBytes   atomic.Int64
	// lastCkSize is what the most recent garbage.php call asked for.
	lastCkSize atomic.Int64
}

func newFakeLibre(t *testing.T) *fakeLibre {
	t.Helper()
	f := &fakeLibre{isp: `{"processedString":"203.0.113.7 - EXAMPLE NET, ` +
		`United States","rawIspInfo":{"asn":"AS64512",` +
		`"as_name":"EXAMPLE NET","country":"US","country_name":"United States"}}`}
	mux := http.NewServeMux()

	mux.HandleFunc("/backend/getIP.php", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, f.isp)
	})

	// One handler for both roles, as the real backend has: empty.php is
	// the ping target on GET and the upload sink on POST.
	mux.HandleFunc("/backend/empty.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.upCalls.Add(1)
			n, _ := io.Copy(io.Discard, r.Body)
			f.upBytes.Add(n)
		} else {
			f.pingCalls.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/backend/garbage.php", func(w http.ResponseWriter, r *http.Request) {
		f.downCalls.Add(1)
		ck, _ := strconv.ParseInt(r.URL.Query().Get("ckSize"), 10, 64)
		f.lastCkSize.Store(ck)
		// The real endpoint serves whole mebibytes. A test that served
		// them too would move 20 MB per run, so this one serves a
		// scaled-down payload: the engine only counts what arrives.
		n := ck * 4096
		f.downBytes.Add(n)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		buf := make([]byte, 4096)
		for n > 0 {
			c := min(int64(len(buf)), n)
			if _, err := w.Write(buf[:c]); err != nil {
				return
			}
			n -= c
		}
	})

	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// config points a run at this fake with the same trimmed-down ladder
// the Cloudflare fake uses.
func (f *fakeLibre) config() Config {
	return Config{
		Backend: BackendLibreSpeed,
		Server: Server{
			Name: "Testville, Nowhere (Fake)", City: "Testville",
			Lat: 12.5, Lng: -34.25,
			// A trailing slash on the origin and a leading one on a
			// path: the join must survive both.
			URL:      f.URL + "/",
			Download: "/backend/garbage.php", Upload: "backend/empty.php",
			Ping: "backend/empty.php", Info: "backend/getIP.php",
		},
		LatencySamples:   3,
		DownStages:       []int64{1 << 20, 2 << 20},
		UpStages:         []int64{50_000},
		MinPhaseDuration: time.Nanosecond,
	}
}

func TestLibreSpeedRunHitsEveryEndpoint(t *testing.T) {
	f := newFakeLibre(t)
	var res *Result
	for ev := range New(f.config()).Run(t.Context()) {
		switch ev.Kind {
		case EventError:
			t.Fatalf("run failed in %v: %v", ev.Phase, ev.Err)
		case EventDone:
			res = ev.Res
		}
	}
	if res == nil {
		t.Fatal("run produced no result")
	}

	if f.pingCalls.Load() < 3 {
		t.Errorf("ping called %d times, want at least 3", f.pingCalls.Load())
	}
	if f.downCalls.Load() != 2 || f.upCalls.Load() != 1 {
		t.Errorf("down/up calls = %d/%d, want 2/1",
			f.downCalls.Load(), f.upCalls.Load())
	}
	if res.DownBytes != f.downBytes.Load() {
		t.Errorf("engine counted %d down bytes, server sent %d",
			res.DownBytes, f.downBytes.Load())
	}
	if res.UpBytes != f.upBytes.Load() {
		t.Errorf("engine counted %d up bytes, server got %d",
			res.UpBytes, f.upBytes.Load())
	}

	// The far-end pin comes from the chosen server, not from the wire:
	// LibreSpeed has no endpoint that reports where the server is.
	if !res.Trace.ColoKnown || res.Trace.ColoCity != "Testville" ||
		res.Trace.ColoLat != 12.5 || res.Trace.ColoLng != -34.25 {
		t.Errorf("server pin = %+v", res.Trace)
	}
	// The client pin is the country centroid, the same coarse lookup
	// the Cloudflare path uses.
	if !res.Trace.ClientKnown || res.Trace.Loc != "US" {
		t.Errorf("client pin = %+v", res.Trace)
	}
	if res.Trace.IP != "203.0.113.7" || res.Trace.ASN != 64512 ||
		res.Trace.ASOrg != "EXAMPLE NET" {
		t.Errorf("client identity = %+v", res.Trace)
	}
}

func TestLibreSpeedSurvivesAServerWithNoIPDatabase(t *testing.T) {
	// Four of the six public servers checked while this was written
	// answer like this. A run against them must still work, with the
	// client pin simply missing.
	f := newFakeLibre(t)
	f.isp = `{"processedString":"203.0.113.7 - Unknown ISP","rawIspInfo":null}`

	var res *Result
	for ev := range New(f.config()).Run(t.Context()) {
		if ev.Kind == EventError {
			t.Fatalf("run failed in %v: %v", ev.Phase, ev.Err)
		}
		if ev.Kind == EventDone {
			res = ev.Res
		}
	}
	if res == nil {
		t.Fatal("run produced no result")
	}
	if res.Trace.IP != "203.0.113.7" {
		t.Errorf("IP = %q", res.Trace.IP)
	}
	// "Unknown ISP" is the endpoint saying it does not know, so it must
	// not reach the connection panel as though it were a network name.
	if res.Trace.ASOrg != "" {
		t.Errorf("ASOrg = %q, want empty", res.Trace.ASOrg)
	}
	if res.Trace.ClientKnown {
		t.Error("client pin claimed to be known with no country")
	}
	// The server pin still works: it never came from the wire.
	if !res.Trace.ColoKnown {
		t.Error("server pin lost along with the client one")
	}
}

func TestLibreSpeedDownloadURL(t *testing.T) {
	cfg := Config{Server: Server{
		URL:      "https://h.example/librespeed/",
		Download: "backend/garbage.php",
	}}
	b := libreSpeedBackend{}

	// Bytes become whole mebibytes, rounded up, so the ladder's small
	// warm-up rung still asks for something.
	for _, c := range []struct{ in, want int64 }{
		{1, 1}, {100_000, 1}, {mib, 1}, {mib + 1, 2},
		{25_000_000, 24}, {1 << 40, maxCkSize},
	} {
		got := b.downloadURL(cfg, c.in)
		want := "https://h.example/librespeed/backend/garbage.php?ckSize=" +
			strconv.FormatInt(c.want, 10)
		if len(got) < len(want) || got[:len(want)] != want {
			t.Errorf("downloadURL(%d) = %q, want prefix %q", c.in, got, want)
		}
	}

	// Every request carries a cache buster, and no two share one: a
	// cached garbage.php response measures the proxy, not the link.
	a, z := b.downloadURL(cfg, mib), b.downloadURL(cfg, mib)
	if a == z {
		t.Errorf("two download URLs were identical: %q", a)
	}
}

func TestParseLatLng(t *testing.T) {
	if lat, lng, ok := parseLatLng(" 52.37 , 4.89 "); !ok ||
		lat != 52.37 || lng != 4.89 {
		t.Errorf("parseLatLng = %v,%v,%v", lat, lng, ok)
	}
	for _, s := range []string{"", "52.37", "a,b", "91,0", "0,181", "NaN,0", "Inf,0"} {
		if _, _, ok := parseLatLng(s); ok {
			t.Errorf("parseLatLng(%q) accepted", s)
		}
	}
}

// identify is the one backend method whose failure ends a run, so its
// error path gets its own test.
func TestLibreSpeedIdentifyRejectsABadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	t.Cleanup(srv.Close)

	cfg := Config{
		Backend: BackendLibreSpeed,
		Server:  Server{URL: srv.URL, Info: "backend/getIP.php"},
	}.withDefaults()
	if _, err := (libreSpeedBackend{}).identify(context.Background(), cfg); err == nil {
		t.Error("identify accepted a 503")
	}
}
