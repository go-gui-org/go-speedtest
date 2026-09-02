package probe

import (
	"net"
	"net/http"
	"time"
)

// DefaultBaseURL is the public Cloudflare speed test origin. Its
// endpoints are undocumented, so every call here treats a failure as
// expected rather than exceptional.
const DefaultBaseURL = "https://speed.cloudflare.com"

// DefaultUserAgent identifies this app to the endpoints it calls. Be a
// good citizen: a demo that hammers a free service anonymously is how
// free services stop being free.
const DefaultUserAgent = "go-speedtest/0.1 (+https://github.com/go-gui-org/go-speedtest)"

// Config holds everything a run needs. The zero value is usable: New
// fills in defaults.
type Config struct {
	// BaseURL is the speed test origin. Overridden by tests with an
	// httptest server.
	BaseURL string

	// Client is the HTTP client for every request. When nil, New builds
	// one that disables compression, because a compressible payload
	// would measure the CPU, not the link.
	Client *http.Client

	// UserAgent is sent on every request.
	UserAgent string

	// LatencySamples is how many round trips to time. Defaults to 30 —
	// enough for a stable p95 and for the histogram to have a shape.
	LatencySamples int

	// DownStages and UpStages are payload sizes in bytes, run in order.
	// Small first: the early stages warm the connection while the chart
	// already has something to draw.
	DownStages []int64
	UpStages   []int64

	// MinPhaseDuration is how long each transfer direction must run.
	// After the ladder is exhausted the largest payload repeats until
	// this much time has passed.
	//
	// A fixed ladder measures a fast link for a fraction of a second
	// and a slow one for a minute. Measuring for a set time instead
	// gives every link the same number of readings, which is what the
	// chart and the percentile both want. Defaults to 10s per
	// direction.
	MinPhaseDuration time.Duration

	// MaxPhaseBytes caps how much one direction may transfer, so a
	// very fast link cannot spend a gigabyte reaching
	// MinPhaseDuration. Defaults to 150 MB.
	//
	// The cap is also a courtesy. The endpoints are free and shared,
	// and they answer a client that asks for too much with 429.
	MaxPhaseBytes int64

	// Timeout bounds one whole run.
	Timeout time.Duration

	// Simulate replaces every network call with the offline generator
	// in sim.go. The UI is identical; only the source of numbers moves.
	Simulate bool

	// SimSpeed scales the simulated run's wall-clock time. 1 is real
	// time, which is what a demo wants; tests set it small so a full
	// scripted run takes milliseconds. Ignored unless Simulate is set.
	SimSpeed float64
}

// Default payload ladders. The ladder only sets the warm-up shape:
// MinPhaseDuration decides when a direction actually stops, by
// repeating the largest payload.
//
// The upload ladder is as long as the download one on purpose. A short
// upload finishes in about a second on a fast link, and a curve drawn
// from four readings looks like noise rather than a measurement.
var (
	defaultDownStages = []int64{100_000, 1_000_000, 10_000_000, 25_000_000}
	defaultUpStages   = []int64{100_000, 1_000_000, 10_000_000, 25_000_000}
)

// withDefaults returns a copy of cfg with every unset field filled.
func (cfg Config) withDefaults() Config {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if len(cfg.UserAgent) > 512 {
		cfg.UserAgent = cfg.UserAgent[:512]
	}
	if cfg.LatencySamples <= 0 {
		cfg.LatencySamples = 30
	}
	if cfg.LatencySamples > 1000 {
		cfg.LatencySamples = 1000
	}
	if cfg.MinPhaseDuration <= 0 {
		cfg.MinPhaseDuration = 8 * time.Second
	}
	if cfg.MaxPhaseBytes <= 0 {
		cfg.MaxPhaseBytes = 150 << 20
	}
	if len(cfg.DownStages) == 0 {
		cfg.DownStages = defaultDownStages
	}
	if len(cfg.UpStages) == 0 {
		cfg.UpStages = defaultUpStages
	}
	// Cap stage lists supplied by callers: unbounded ladders drive
	// unbounded allocations and request counts.
	if len(cfg.DownStages) > 64 {
		cfg.DownStages = cfg.DownStages[:64]
	}
	if len(cfg.UpStages) > 64 {
		cfg.UpStages = cfg.UpStages[:64]
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.SimSpeed <= 0 || cfg.SimSpeed != cfg.SimSpeed {
		cfg.SimSpeed = 1
	}
	if cfg.Client == nil {
		cfg.Client = newClient()
	}
	return cfg
}

// newClient builds the measuring HTTP client.
//
// DisableCompression matters: the __down endpoint returns zero bytes,
// which gzip crushes to nothing. Measuring that would report a link
// speed of several hundred Gbit/s.
func newClient() *http.Client {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DisableCompression:  true,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        8,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: tr}
}
