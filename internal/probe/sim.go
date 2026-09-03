package probe

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// The offline demo generator.
//
// It exists for three reasons: the app must be demonstrable with no
// network, the screenshot mode needs numbers that do not depend on
// whoever runs it, and a live demo on a conference wifi should not be a
// coin toss. It runs in real time and emits exactly the same events as
// a real run, so the UI cannot tell the difference and nothing in the
// view layer needs a demo branch.

// Simulated run shape. Chosen to look like a good home fibre line.
const (
	simDownPeak = 465.0 // Mbps plateau
	simUpPeak   = 118.0 // Mbps plateau
	simBaseRTT  = 24 * time.Millisecond
	simDownTime = 10 * time.Second
	simUpTime   = 10 * time.Second
)

// runSimulated plays a scripted run against the clock.
func (e *Engine) runSimulated(ctx context.Context, emit func(Event)) {
	// A fixed seed keeps screenshots reproducible. The wobble still
	// looks organic; it is just the same organic every time.
	rng := rand.New(rand.NewPCG(0x5EED, 0xC0FFEE))
	scale := e.cfg.SimSpeed
	started := time.Now()
	res := &Result{Simulated: true}

	emit(Event{Kind: EventPhase, Phase: PhaseTrace})
	if !simSleep(ctx, scaled(350*time.Millisecond, scale)) {
		return
	}
	// Documentation-range address and a made-up network, so a demo
	// screenshot never shows a real subscriber's details.
	tr := &Trace{
		Colo: "SEA", IP: "203.0.113.7", Loc: "US",
		ASN: 64512, ASOrg: "EXAMPLE VALLEY BROADBAND COOPERATIVE",
		City: "Peosta", Region: "Iowa", HTTPProtocol: "HTTP/2",
	}
	resolveLocations(tr)
	res.Trace = *tr
	emit(Event{Kind: EventTrace, Phase: PhaseTrace, Trace: tr})

	// Latency: a stable base with small jitter, plus the occasional
	// outlier so the box plot has whiskers and dots to draw.
	emit(Event{Kind: EventPhase, Phase: PhaseLatency})
	latStart := time.Now()
	for i := 0; i < e.cfg.LatencySamples; i++ {
		if !simSleep(ctx, scaled(60*time.Millisecond, scale)) {
			return
		}
		rtt := simBaseRTT + time.Duration(rng.NormFloat64()*2.5)*time.Millisecond
		if rng.Float64() < 0.08 {
			// One sample in twelve hits a retransmit or a busy queue.
			rtt += time.Duration(10+rng.IntN(40)) * time.Millisecond
		}
		if rtt < time.Millisecond {
			rtt = time.Millisecond
		}
		res.RTTs = append(res.RTTs, rtt)
		emit(Event{
			Kind:    EventRTT,
			Phase:   PhaseLatency,
			Elapsed: time.Since(latStart),
			RTT:     rtt,
		})
	}

	// Download and upload: a ramp into a noisy plateau.
	emit(Event{Kind: EventPhase, Phase: PhaseDownload})
	downReadings, downBytes := simTransfer(ctx, emit, rng, PhaseDownload, simDownPeak, simDownTime, scale)
	res.DownBytes = downBytes
	res.DownMbps = headline(downReadings)
	if contextDone(ctx) {
		return
	}

	emit(Event{Kind: EventPhase, Phase: PhaseUpload})
	upReadings, upBytes := simTransfer(ctx, emit, rng, PhaseUpload, simUpPeak, simUpTime, scale)
	res.UpBytes = upBytes
	// The generator produces honest readings by construction — there is
	// no socket buffer to hide behind — so the same percentile rule the
	// download uses applies here.
	res.UpMbps = headline(upReadings)
	if contextDone(ctx) {
		return
	}

	res.Duration = time.Since(started)
	emit(Event{Kind: EventPhase, Phase: PhaseDone})
	emit(Event{Kind: EventDone, Phase: PhaseDone, Res: res})
}

// simTransfer emits rate events for one direction over the given
// duration, then a stage event, and returns the readings and the byte
// total they imply.
func simTransfer(ctx context.Context, emit func(Event), rng *rand.Rand, phase Phase, peak float64, dur time.Duration, scale float64) ([]float64, int64) {
	tick := scaled(rateInterval, scale)
	dur = scaled(dur, scale)
	var (
		readings []float64
		bytes    float64
		start    = time.Now()
	)
	for {
		if !simSleep(ctx, tick) {
			return readings, int64(bytes)
		}
		elapsed := time.Since(start)
		if elapsed >= dur {
			break
		}

		mbps := simRate(peak, elapsed, dur, rng)
		readings = append(readings, mbps)
		bytes += mbps * 1e6 / 8 * tick.Seconds()
		emit(Event{
			Kind:    EventRate,
			Phase:   phase,
			Elapsed: elapsed,
			Mbps:    mbps,
		})
	}

	emit(Event{
		Kind:    EventStage,
		Phase:   phase,
		Elapsed: time.Since(start),
		Mbps:    stats.Mean(readings),
		Bytes:   int64(bytes),
	})
	return readings, int64(bytes)
}

// simRate models one instantaneous reading: an exponential ramp into a
// plateau, wobble on top, and a rare dip to stand in for congestion.
func simRate(peak float64, elapsed, total time.Duration, rng *rand.Rand) float64 {
	if peak != peak || peak < 0 {
		return 0
	}
	if total <= 0 {
		return 0
	}
	// Ramp reaches about 95% of peak a fifth of the way in, which is
	// roughly how TCP slow start looks on a short transfer.
	ramp := 1 - math.Exp(-5*elapsed.Seconds()/(total.Seconds()*0.2))
	v := peak * ramp
	v *= 1 + rng.NormFloat64()*0.04
	if rng.Float64() < 0.03 {
		v *= 0.55
	}
	if v != v || v < 0 {
		v = 0
	}
	return v
}

// scaled shortens a scripted duration by the configured factor, with a
// floor so a very small factor does not turn every wait into a busy
// loop.
func scaled(d time.Duration, factor float64) time.Duration {
	if factor != factor || factor <= 0 {
		factor = 1
	}
	if d < 0 {
		d = 0
	}
	return max(time.Duration(float64(d)*factor), time.Millisecond)
}

// simSleep waits for d and reports whether the run should continue.
func simSleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
