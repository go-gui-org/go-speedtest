// Package probe measures network performance. It imports no UI package
// and never touches a window: the whole engine is plain Go over
// net/http, so it can be tested against an httptest server in
// milliseconds and reused from the CLI report mode.
package probe

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// eventBuffer sizes the channel between the engine goroutine and the
// UI. Large enough that a slow frame never stalls a transfer, small
// enough that the UI cannot fall a whole phase behind.
const eventBuffer = 256

// Engine runs one speed test and streams its progress.
type Engine struct {
	cfg Config
}

// New returns an Engine with cfg's unset fields defaulted.
func New(cfg Config) *Engine {
	return &Engine{cfg: cfg.withDefaults()}
}

// Config returns the effective configuration, defaults filled in.
func (e *Engine) Config() Config { return e.cfg }

// Run starts a test and returns the event stream.
//
// The engine owns a goroutine for the run and closes the channel when
// it finishes. The last event is always EventDone or EventError. The
// caller must drain the channel; a caller that walks away should cancel
// ctx, or the goroutine blocks on a full buffer until the run's own
// timeout fires.
func (e *Engine) Run(ctx context.Context) <-chan Event {
	out := make(chan Event, eventBuffer)
	go func() {
		defer close(out)
		e.run(ctx, func(ev Event) { out <- ev })
	}()
	return out
}

// run executes the phases in order. Any failure ends the run: a speed
// test that could not download has nothing useful to say about upload.
func (e *Engine) run(ctx context.Context, emit func(Event)) {
	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	if e.cfg.Simulate {
		e.runSimulated(ctx, emit)
		return
	}

	started := time.Now()
	res := &Result{}

	// Phase 1: where are we, and who is answering.
	emit(Event{Kind: EventPhase, Phase: PhaseTrace})
	tr, err := fetchTrace(ctx, e.cfg)
	if err != nil {
		fail(emit, PhaseTrace, err)
		return
	}
	// Best effort, and deliberately not fatal: this call only adds
	// detail to the connection panel, and the run below does not
	// depend on any of it.
	if err := fetchMeta(ctx, e.cfg, tr); err != nil {
		slog.Debug("meta lookup failed", "err", err)
	}
	res.Trace = *tr
	emit(Event{Kind: EventTrace, Phase: PhaseTrace, Trace: tr})

	// Phase 2: idle latency, the baseline the loaded samples in the
	// transfer phases are compared against.
	emit(Event{Kind: EventPhase, Phase: PhaseLatency})
	latStart := time.Now()
	var lastErr error
	for i := 0; i < e.cfg.LatencySamples; i++ {
		if contextDone(ctx) {
			fail(emit, PhaseLatency, ctx.Err())
			return
		}
		rtt, rerr := measureRTT(ctx, e.cfg)
		if rerr != nil {
			// One lost sample is normal on a busy link. Only give up if
			// every sample failed, which the check after the loop does.
			lastErr = rerr
			continue
		}
		res.RTTs = append(res.RTTs, rtt)
		emit(Event{
			Kind:    EventRTT,
			Phase:   PhaseLatency,
			Elapsed: time.Since(latStart),
			RTT:     rtt,
		})
	}
	if len(res.RTTs) == 0 {
		// Report why, when the probes said why. "No samples" alone
		// sends the reader looking at their link when the endpoint was
		// turning them away.
		if lastErr == nil {
			lastErr = errNoSamples
		}
		fail(emit, PhaseLatency, lastErr)
		return
	}

	// Phase 3: download. Latency is sampled throughout: idle latency
	// next to latency under load is the measurement that says whether
	// the link is usable while it is busy.
	emit(Event{Kind: EventPhase, Phase: PhaseDownload})
	downSampler := startRTTSampler(ctx, e.cfg, PhaseDownload, emit)
	downReadings, downBytes, err := runDownload(ctx, e.cfg, time.Now(), emit)
	res.DownRTTs = downSampler.stop()
	res.DownBytes = downBytes
	res.DownMbps = headline(downReadings)
	if err != nil {
		// A phase that already moved bytes is not a failed run. A
		// connection dropped part way through a download says nothing
		// about the upload, and discarding both would throw away a
		// good measurement. The test is bytes transferred, not chart
		// readings: a payload can complete before the first reading is
		// due.
		if downBytes == 0 || ctx.Err() != nil {
			fail(emit, PhaseDownload, err)
			return
		}
		emit(Event{Kind: EventWarning, Phase: PhaseDownload, Err: err})
	}

	// Phase 4: upload.
	emit(Event{Kind: EventPhase, Phase: PhaseUpload})
	upSampler := startRTTSampler(ctx, e.cfg, PhaseUpload, emit)
	_, upBytes, upStages, err := runUpload(ctx, e.cfg, time.Now(), emit)
	res.UpRTTs = upSampler.stop()
	res.UpBytes = upBytes
	res.UpMbps = uploadHeadline(upStages)
	if err != nil {
		if upBytes == 0 || ctx.Err() != nil {
			fail(emit, PhaseUpload, err)
			return
		}
		emit(Event{Kind: EventWarning, Phase: PhaseUpload, Err: err})
	}

	res.Duration = time.Since(started)
	emit(Event{Kind: EventPhase, Phase: PhaseDone})
	emit(Event{Kind: EventDone, Phase: PhaseDone, Res: res})
}

// headline picks the number to show as "your speed".
//
// The 90th percentile of the instantaneous readings, not the mean: the
// first second of every transfer is TCP ramping up, and averaging that
// in reports a speed the link never actually reached its peak at. p90
// answers "how fast does this connection go when it is going", which is
// the question being asked.
func headline(readings []float64) float64 {
	return stats.Percentile(readings, 0.90)
}

// uploadHeadline picks the upload number to report.
//
// The largest completed stage, measured end to end. Buffering makes the
// live readings unusable for this: on a fast machine a whole payload
// can be handed to the kernel long before it reaches the wire. The
// largest stage is the one where transfer time dominates connection
// setup, so it is the least distorted measurement available from the
// client side.
func uploadHeadline(stages []stageStat) float64 {
	var best stageStat
	for _, s := range stages {
		if s.Bytes > best.Bytes {
			best = s
		}
	}
	return best.Mbps()
}

// fail emits the error and the terminal phase change.
func fail(emit func(Event), p Phase, err error) {
	emit(Event{Kind: EventError, Phase: p, Err: err})
	emit(Event{Kind: EventPhase, Phase: PhaseError})
}
