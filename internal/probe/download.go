package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// rateInterval is how often a transfer reports an instantaneous rate.
// It sets the resolution of the live area chart: short enough that a
// two-second stage still draws a curve rather than a zigzag, long
// enough that one slow chunk does not make the line spike.
const rateInterval = 50 * time.Millisecond

// downloadChunk is the read buffer size. Large enough that the syscall
// cost disappears, small enough that a fast link still produces several
// reads per rateInterval.
const downloadChunk = 64 << 10

// runDownload pulls payloads until the phase has run for
// cfg.MinPhaseDuration, reporting an instantaneous rate roughly every
// rateInterval and a mean rate at the end of each payload.
//
// phaseStart is the reference for Event.Elapsed, so the chart's X axis
// covers the whole phase rather than restarting per stage.
//
// It returns every instantaneous reading, for the percentile summary.
func runDownload(ctx context.Context, cfg Config, phaseStart time.Time, emit func(Event)) ([]float64, int64, error) {
	var (
		readings []float64
		total    int64
	)
	for size := range ladder(cfg.DownStages, cfg.MinPhaseDuration, cfg.MaxPhaseBytes, phaseStart, &total) {
		got, stageReadings, err := downloadOne(ctx, cfg, size, phaseStart, emit)
		total += got
		readings = append(readings, stageReadings...)
		if err != nil {
			return readings, total, err
		}
	}
	return readings, total, nil
}

// maxLadderRepeats bounds how many times the largest payload repeats.
//
// It is a safety valve, not a tuning knob. Every real transfer takes
// time, so the duration check stops the ladder long before this. An
// endpoint that answers instantly with no body would otherwise spin
// this loop until the run timed out.
const maxLadderRepeats = 200

// ladder yields payload sizes for one direction: the configured stages
// in order, then the largest one repeated until the phase has run long
// enough.
//
// It reads *sent after each yield, so the caller's running total decides
// when the byte cap is reached. The iterator stops on the first of three
// conditions: the time is up, the cap is reached, or (for a caller that
// passes no stages) there is nothing to send.
func ladder(stages []int64, minDuration time.Duration, maxBytes int64, phaseStart time.Time, sent *int64) func(func(int64) bool) {
	return func(yield func(int64) bool) {
		if len(stages) == 0 {
			return
		}
		for _, size := range stages {
			if *sent >= maxBytes {
				return
			}
			if !yield(size) {
				return
			}
		}
		// Repeat the largest payload until the phase has lasted long
		// enough. Largest, not the next one up: it is the size where
		// transfer time dominates request setup.
		largest := stages[len(stages)-1]
		for range maxLadderRepeats {
			if time.Since(phaseStart) >= minDuration || *sent >= maxBytes {
				return
			}
			if !yield(largest) {
				return
			}
		}
	}
}

// downloadOne fetches a single payload of the requested size.
func downloadOne(ctx context.Context, cfg Config, size int64, phaseStart time.Time, emit func(Event)) (int64, []float64, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/__down?bytes=" + strconv.FormatInt(size, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)
	// Belt and braces: the transport disables compression, but an
	// intermediary might still try.
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("download request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("download: unexpected status %s", resp.Status)
	}

	var (
		buf       = make([]byte, downloadChunk)
		stageStat = time.Now()
		windowAt  = stageStat
		windowN   int64
		total     int64
		readings  []float64
	)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			windowN += int64(n)
			if since := time.Since(windowAt); since >= rateInterval {
				mbps := toMbps(windowN, since)
				readings = append(readings, mbps)
				emit(Event{
					Kind:    EventRate,
					Phase:   PhaseDownload,
					Elapsed: time.Since(phaseStart),
					Mbps:    mbps,
				})
				windowAt = time.Now()
				windowN = 0
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return total, readings, fmt.Errorf("download read: %w", rerr)
		}
		// Honour cancellation between chunks; the context also aborts
		// the read itself, this just makes the exit prompt.
		if ctx.Err() != nil {
			return total, readings, ctx.Err()
		}
	}

	elapsed := time.Since(stageStat)
	emit(Event{
		Kind:    EventStage,
		Phase:   PhaseDownload,
		Elapsed: time.Since(phaseStart),
		Mbps:    toMbps(total, elapsed),
		Bytes:   total,
	})
	return total, readings, nil
}

// toMbps converts a byte count over a duration into megabits per
// second. Megabits, base ten, because that is how links are sold.
func toMbps(bytes int64, d time.Duration) float64 {
	if d <= 0 || bytes <= 0 {
		return 0
	}
	v := float64(bytes) * 8 / d.Seconds() / 1e6
	if v != v || v < 0 || v > 1e12 {
		return 0
	}
	return v
}
