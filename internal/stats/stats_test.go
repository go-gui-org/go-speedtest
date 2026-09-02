package stats

import (
	"math"
	"testing"
)

const eps = 1e-9

func approx(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestPercentileInterpolates(t *testing.T) {
	// Ranks 0..4 over five samples. p=0.25 lands on rank 1.0 exactly.
	v := []float64{10, 20, 30, 40, 50}
	approx(t, Percentile(v, 0), 10, "p0")
	approx(t, Percentile(v, 0.25), 20, "p25")
	approx(t, Percentile(v, 0.5), 30, "p50")
	approx(t, Percentile(v, 1), 50, "p100")

	// p=0.9 lands on rank 3.6: 40 + 0.6*(50-40).
	approx(t, Percentile(v, 0.9), 46, "p90")
}

func TestPercentileDoesNotSortInput(t *testing.T) {
	v := []float64{5, 1, 3}
	_ = Percentile(v, 0.5)
	if v[0] != 5 || v[1] != 1 || v[2] != 3 {
		t.Fatalf("input was modified: %v", v)
	}
}

func TestPercentileEmpty(t *testing.T) {
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil) = %v, want 0", got)
	}
}

func TestPercentileSingle(t *testing.T) {
	v := []float64{7}
	for _, p := range []float64{0, 0.5, 0.95, 1} {
		approx(t, Percentile(v, p), 7, "single")
	}
}

func TestMeanStdDev(t *testing.T) {
	v := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	approx(t, Mean(v), 5, "mean")
	approx(t, StdDev(v), 2, "stddev") // population sigma is exactly 2
	approx(t, StdDev([]float64{3}), 0, "stddev of one")
}

func TestMinMax(t *testing.T) {
	v := []float64{3, -1, 9, 0}
	approx(t, Min(v), -1, "min")
	approx(t, Max(v), 9, "max")
	approx(t, Min(nil), 0, "min nil")
	approx(t, Max(nil), 0, "max nil")
}

func TestJitterIsMeanAbsoluteDelta(t *testing.T) {
	// Deltas: 10, 10, 10 -> mean 10.
	approx(t, Jitter([]float64{10, 20, 30, 40}), 10, "ramp")

	// Same spread, alternating: deltas 40, 40, 40 -> mean 40.
	approx(t, Jitter([]float64{10, 50, 10, 50}), 40, "alternating")

	// A slow drift and a fast alternation can share a standard
	// deviation while differing in jitter. That is the whole point of
	// using IPDV, so pin it.
	drift := []float64{10, 20, 30, 40, 50, 60}
	alt := []float64{10, 60, 10, 60, 10, 60}
	if Jitter(alt) <= Jitter(drift) {
		t.Errorf("alternating jitter %v should exceed drift %v",
			Jitter(alt), Jitter(drift))
	}
	approx(t, Jitter([]float64{5}), 0, "single")
	approx(t, Jitter(nil), 0, "nil")
}

func TestHardenedStats(t *testing.T) {
	if got := Percentile([]float64{1, 2, 3}, math.NaN()); got != 0 {
		t.Errorf("Percentile(NaN) = %v, want 0", got)
	}
	// NaN/Inf in the trace must not poison the summary.
	if got := Jitter([]float64{10, math.NaN(), 30}); got != 0 {
		t.Errorf("Jitter with NaN = %v, want 0", got)
	}
	if got := Jitter([]float64{10, math.Inf(1), 30}); got != 0 {
		t.Errorf("Jitter with Inf = %v, want 0", got)
	}
	// Single valid delta amid bad values.
	if got := Jitter([]float64{10, 20, math.NaN()}); got != 5 {
		t.Errorf("Jitter with trailing NaN = %v, want 5", got)
	}
}
