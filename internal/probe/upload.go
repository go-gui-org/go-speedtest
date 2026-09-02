package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// stageStat is one completed payload, measured across the whole
// request including the response. Unlike an instantaneous reading it
// cannot be inflated by buffering, because the clock does not stop
// until the far end has answered.
type stageStat struct {
	Bytes int64
	Dur   time.Duration
}

// Mbps is the mean rate over the stage.
func (s stageStat) Mbps() float64 {
	if s.Bytes < 0 {
		return 0
	}
	return toMbps(s.Bytes, s.Dur)
}

// runUpload pushes payloads until the phase has run for
// cfg.MinPhaseDuration.
//
// Honest caveat, stated once here and repeated in the README: an upload
// rate measured on the client counts bytes handed to the kernel, not
// bytes the far end acknowledged. The headline number therefore comes
// from the stage stats, which time a whole request including its
// response.
//
// The live readings are a running average over the stage rather than a
// per-window rate, for the same reason. A window measurement of a
// buffered write alternates between the speed of memcpy and zero: the
// buffer accepts a burst, then blocks. On a 100 Mbit link that draws
// spikes above 700 Mbps between troughs at nothing, which is noise
// dressed as data. A running average converges on the real rate as the
// buffer stops mattering.
func runUpload(ctx context.Context, cfg Config, phaseStart time.Time, emit func(Event)) ([]float64, int64, []stageStat, error) {
	var (
		readings []float64
		stages   []stageStat
		total    int64
	)
	for size := range ladder(cfg.UpStages, cfg.MinPhaseDuration, cfg.MaxPhaseBytes, phaseStart, &total) {
		sent, stageReadings, st, err := uploadOne(ctx, cfg, size, phaseStart, total, emit)
		total += sent
		readings = append(readings, stageReadings...)
		if st.Bytes > 0 {
			stages = append(stages, st)
		}
		if err != nil {
			return readings, total, stages, err
		}
	}
	return readings, total, stages, nil
}

// uploadOne posts a single payload of the requested size.
func uploadOne(ctx context.Context, cfg Config, size int64, phaseStart time.Time, phaseSent int64, emit func(Event)) (int64, []float64, stageStat, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/__up"

	body := &progressReader{
		remaining: size,
		chunk:     make([]byte, downloadChunk),
		// The running average spans the whole phase, not this payload.
		// Restarting it per payload would replay the buffer inflation
		// at the start of every stage, which draws a sawtooth.
		start:     phaseStart,
		priorSent: phaseSent,
		onRate: func(mbps float64) {
			emit(Event{
				Kind:    EventRate,
				Phase:   PhaseUpload,
				Elapsed: time.Since(phaseStart),
				Mbps:    mbps,
			})
		},
	}
	body.reportedAt = time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return 0, nil, stageStat{}, err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)
	req.Header.Set("Content-Type", "application/octet-stream")
	// Setting ContentLength keeps the request out of chunked encoding,
	// so the byte count on the wire matches the payload size.
	req.ContentLength = size

	stageStart := time.Now()
	resp, err := cfg.Client.Do(req)
	if err != nil {
		return body.sent, body.readings, stageStat{}, fmt.Errorf("upload request: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return body.sent, body.readings, stageStat{}, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return body.sent, body.readings, stageStat{}, fmt.Errorf("upload: unexpected status %s", resp.Status)
	}

	st := stageStat{Bytes: body.sent, Dur: time.Since(stageStart)}
	emit(Event{
		Kind:    EventStage,
		Phase:   PhaseUpload,
		Elapsed: time.Since(phaseStart),
		Mbps:    st.Mbps(),
		Bytes:   st.Bytes,
	})
	return body.sent, body.readings, st, nil
}

// progressReader generates the upload payload on the fly and reports a
// rate every rateInterval. Generating beats holding a 10 MB buffer, and
// the content is irrelevant: the endpoint discards it.
type progressReader struct {
	remaining int64
	chunk     []byte
	sent      int64
	// start is when the phase began and priorSent is what earlier
	// payloads in it already sent; together they make the running
	// average span the phase. reportedAt only paces how often a
	// reading is emitted.
	start      time.Time
	priorSent  int64
	reportedAt time.Time
	readings   []float64
	onRate     func(mbps float64)
}

// Read fills p with as much of the remaining payload as fits.
func (r *progressReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 || len(p) == 0 {
		if r.remaining <= 0 {
			return 0, io.EOF
		}
		return 0, nil
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	// The payload is whatever the zero-valued chunk holds. Never copy
	// real memory contents into an outbound request.
	if n > len(r.chunk) {
		n = len(r.chunk)
	}
	if n <= 0 {
		return 0, nil
	}
	copy(p[:n], r.chunk[:n])

	r.remaining -= int64(n)
	r.sent += int64(n)
	if time.Since(r.reportedAt) >= rateInterval {
		// Running average over the whole stage, not the last window.
		// See the note on runUpload for why.
		mbps := toMbps(r.priorSent+r.sent, time.Since(r.start))
		r.readings = append(r.readings, mbps)
		if r.onRate != nil {
			r.onRate(mbps)
		}
		r.reportedAt = time.Now()
	}
	return n, nil
}

// contextDone is a small helper used by the engine to stop between
// stages without threading a select through every call.
func contextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
