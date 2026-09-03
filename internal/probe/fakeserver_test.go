package probe

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEdge stands in for speed.cloudflare.com. It answers the three
// endpoints the engine uses and counts what it was asked for, so tests
// can assert the engine's request pattern rather than its timings.
type fakeEdge struct {
	*httptest.Server

	colo string
	loc  string

	traceCalls atomic.Int64
	downCalls  atomic.Int64
	upCalls    atomic.Int64
	downBytes  atomic.Int64
	upBytes    atomic.Int64

	// downStatus, when non-zero, is returned by every /__down call.
	downStatus atomic.Int64
	// failDownAfter, when non-zero, makes /__down fail once this many
	// calls have already succeeded. It reproduces a link that drops
	// part way through a transfer.
	failDownAfter atomic.Int64

	metaCalls atomic.Int64
}

func newFakeEdge(t *testing.T) *fakeEdge {
	t.Helper()
	e := &fakeEdge{colo: "SEA", loc: "US"}
	mux := http.NewServeMux()

	mux.HandleFunc("/cdn-cgi/trace", func(w http.ResponseWriter, r *http.Request) {
		e.traceCalls.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		// Includes keys the parser must ignore, and lower-case values
		// the parser must upper-case.
		io.WriteString(w, "fl=99f42\nh=speed.example\nip=203.0.113.7\n"+
			"ts=1700000000.1\nvisit_scheme=https\ncolo="+
			toLowerASCII(e.colo)+"\nloc="+toLowerASCII(e.loc)+"\nhttp=http/2\n")
	})

	mux.HandleFunc("/meta", func(w http.ResponseWriter, r *http.Request) {
		e.metaCalls.Add(1)
		// Carries a field the decoder must ignore, so the endpoint can
		// grow without breaking the parse.
		w.Header().Set("Content-Type", "application/json")
		// The colo block is filled only for the one code this fake
		// knows, so a test that sets an unrecognised colo still gets
		// the unresolved case it is asking for.
		colo := `{"iata":"` + e.colo + `"}`
		if e.colo == "SEA" {
			colo = `{"iata":"SEA","city":"Seattle","lat":47.45,"lon":-122.31}`
		}
		io.WriteString(w, `{"clientIp":"203.0.113.7","httpProtocol":"HTTP/2",`+
			`"asn":64512,"asOrganization":"EXAMPLE NET","city":"Peosta",`+
			`"region":"Iowa","country":"US","postalCode":"52068",`+
			`"colo":`+colo+`}`)
	})

	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		calls := e.downCalls.Add(1)
		n, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)

		if st := e.downStatus.Load(); st != 0 {
			w.WriteHeader(int(st))
			return
		}
		// failDownAfter counts payload requests only. The latency probe
		// asks for zero bytes on the same path and must keep working,
		// or the test would be exercising a failed latency phase
		// instead of a failed download.
		if limit := e.failDownAfter.Load(); limit != 0 && n > 0 && calls > limit {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		e.downBytes.Add(n)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		// Write in chunks so the reader sees more than one Read.
		buf := make([]byte, 32<<10)
		for n > 0 {
			c := min(int64(len(buf)), n)
			if _, err := w.Write(buf[:c]); err != nil {
				return
			}
			n -= c
		}
	})

	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		e.upCalls.Add(1)
		n, _ := io.Copy(io.Discard, r.Body)
		e.upBytes.Add(n)
		w.WriteHeader(http.StatusNoContent)
	})

	e.Server = httptest.NewServer(mux)
	t.Cleanup(e.Close)
	return e
}

// config returns a Config pointed at this server with small payloads
// and no minimum phase duration, so each ladder runs exactly once and
// the whole engine test finishes in milliseconds.
func (e *fakeEdge) config() Config {
	return Config{
		BaseURL:          e.URL,
		LatencySamples:   3,
		DownStages:       []int64{50_000, 100_000},
		UpStages:         []int64{50_000},
		MinPhaseDuration: time.Nanosecond,
	}
}

// toLowerASCII lower-cases without pulling in strings, keeping the fake
// server's intent obvious: the parser, not this helper, is under test.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
