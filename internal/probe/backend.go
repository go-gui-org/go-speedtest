package probe

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Backend names the wire protocol a provider speaks.
//
// Until now every provider was a base URL, because every provider was
// Cloudflare's speed worker under a different origin. LibreSpeed is a
// genuinely different protocol — different paths, a different way of
// asking for a payload size, and a different endpoint for "who is at
// each end" — so the engine needs to know which one it is talking to.
//
// It is an enum rather than an interface on Config because Config is a
// plain data struct that tests build by hand; an unexported interface
// field would make it unbuildable from outside the package, and an
// exported one would invite callers to supply their own.
type Backend uint8

const (
	// BackendCloudflare is speed.cloudflare.com's worker API, and the
	// zero value: a Config that says nothing about a backend gets the
	// one this app started with.
	BackendCloudflare Backend = iota
	// BackendLibreSpeed is the open LibreSpeed backend, as run by the
	// servers in the public list.
	BackendLibreSpeed
)

// backend is the per-protocol behaviour the phases need: where to send
// each request, and how to learn who is at each end of the link.
//
// Only four methods, because only four things actually differ. The
// measurement itself — the ladder, the rate windows, the RTT sampler,
// the percentile — is protocol-independent and stays shared.
type backend interface {
	// downloadURL asks for a payload of roughly size bytes.
	downloadURL(cfg Config, size int64) string
	// uploadURL is the sink a payload is POSTed to.
	uploadURL(cfg Config) string
	// pingURL answers with an empty body, for the round-trip timing.
	pingURL(cfg Config) string
	// identify fills in the trace: the client's address and country,
	// and where the far end is. A failure here ends the run, because
	// every later phase talks to the same host.
	identify(ctx context.Context, cfg Config) (*Trace, error)
}

// resolveBackend returns the implementation for cfg's Backend. An
// unknown value falls back to Cloudflare rather than panicking: a bad
// enum should degrade to the default provider, not kill the window.
func (cfg Config) resolveBackend() backend {
	switch cfg.Backend {
	case BackendLibreSpeed:
		return libreSpeedBackend{}
	default:
		return cloudflareBackend{}
	}
}

// tune applies the protocol's request budget to cfg, leaving any field
// the caller already set alone.
//
// The measurement is the same shape for every provider, but the hosts
// are not. Cloudflare's endpoint is built to be hammered; several of
// the public LibreSpeed servers are donated bandwidth behind a firewall
// that starts refusing a client that asks too often. So how many
// requests a run makes is a property of the provider, not of the app.
//
// Only unset fields are filled, so a test that pins a ladder keeps it.
func (b Backend) tune(cfg Config) Config {
	if b != BackendLibreSpeed {
		return cfg
	}
	if cfg.LatencySamples == 0 {
		// Ten is still enough for a median and a jitter figure, and it
		// is a third of the requests the default asks for.
		cfg.LatencySamples = 10
	}
	if cfg.LoadedRTTInterval == 0 {
		// Halving the sampling rate halves the probe requests during
		// the two transfer phases, which is where most of a run's
		// request count lives.
		cfg.LoadedRTTInterval = time.Second
	}
	if len(cfg.DownStages) == 0 {
		// A longer top rung means fewer repeats to fill the phase.
		// The rungs stay in the same range as the Cloudflare ladder,
		// because a payload is not interruptible: a much larger one
		// would run well past MinPhaseDuration on a slow link.
		cfg.DownStages = []int64{1 << 20, 4 << 20, 16 << 20, 32 << 20}
	}
	if len(cfg.UpStages) == 0 {
		// The upload ladder stops at 4 MiB, well short of the download
		// one, because the sink is a PHP script and PHP caps the
		// request body it will accept. Two of the servers checked
		// while this was written answer 413 to anything over about 5
		// MiB, and the rest take 24 MiB happily; 4 MiB is the size
		// every one of them accepts.
		//
		// The cost is more requests to fill the phase. That is the
		// right way round: a refused payload loses the whole stage,
		// while an extra request costs one round trip.
		cfg.UpStages = []int64{512 << 10, 1 << 20, 2 << 20, 4 << 20}
	}
	if cfg.MaxPhaseBytes == 0 {
		// A courtesy cap, two thirds of the default. These servers are
		// donated, and a showcase app has no business moving 150 MB in
		// each direction through one.
		cfg.MaxPhaseBytes = 100 << 20
	}
	return cfg
}

// cloudflareBackend speaks speed.cloudflare.com's worker API.
type cloudflareBackend struct{}

func (cloudflareBackend) downloadURL(cfg Config, size int64) string {
	return trimBase(cfg.BaseURL) + "/__down?bytes=" +
		strconv.FormatInt(size, 10)
}

func (cloudflareBackend) uploadURL(cfg Config) string {
	return trimBase(cfg.BaseURL) + "/__up"
}

func (cloudflareBackend) pingURL(cfg Config) string {
	return trimBase(cfg.BaseURL) + "/__down?bytes=0"
}

// identify runs the trace and then the meta lookup.
//
// The meta failure is swallowed on purpose and reported by the caller
// at debug level: by that point the trace has already succeeded, so a
// lost meta call is a shorter connection panel rather than a failed
// run.
func (cloudflareBackend) identify(ctx context.Context, cfg Config) (*Trace, error) {
	t, err := fetchTrace(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := fetchMeta(ctx, cfg, t); err != nil {
		return t, metaFailed{err}
	}
	return t, nil
}

// metaFailed marks a cosmetic identify failure: the trace is usable,
// only the extra detail is missing. The engine logs it and carries on.
type metaFailed struct{ err error }

func (m metaFailed) Error() string { return m.err.Error() }
func (m metaFailed) Unwrap() error { return m.err }

// trimBase drops a trailing slash so appending a rooted path does not
// produce a double slash.
func trimBase(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

// joinPath glues a server origin to one of its endpoint paths. The
// public LibreSpeed list mixes the two shapes — some entries put the
// backend directory in the origin and name "garbage.php", others keep
// a bare origin and name "backend/garbage.php" — so both halves are
// trimmed and one slash is put back.
func joinPath(base, path string) string {
	return trimBase(base) + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
}

// cacheBust is a counter appended to requests that must not be served
// from a cache.
//
// A proxy that answers a garbage.php request from its cache reports the
// speed of the proxy, not of the link. The upstream LibreSpeed client
// appends a random value for the same reason; a counter is enough here
// and costs no entropy source.
var cacheBust atomic.Uint64

// bust returns the next cache-busting value as a string.
func bust() string {
	return strconv.FormatUint(cacheBust.Add(1), 36)
}
