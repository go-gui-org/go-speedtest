// Package stats holds the small numeric helpers the speed test needs.
//
// It exists because go-charts has no exported quantile helper: the box
// plot computes Tukey stats internally and does not expose them. Rather
// than reach into that package, the app owns these few functions, keeps
// them tested, and feeds go-charts the raw samples it wants.
package stats

import (
	"math"
	"sort"
)

// Percentile returns the p-th percentile of vals using linear
// interpolation between the two nearest ranks. p is a fraction in
// [0, 1]; 0.95 means p95.
//
// Linear interpolation, rather than nearest-rank, so that a p95 taken
// over 20 samples moves smoothly as samples arrive instead of jumping
// between two fixed values.
//
// The input is not modified: vals is copied before sorting.
func Percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	// NaN percentiles must not propagate into gauge or header math.
	if p != p {
		return 0
	}
	if p <= 0 {
		return Min(vals)
	}
	if p >= 1 {
		return Max(vals)
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)

	// Rank in [0, n-1]. The integer part picks the lower sample, the
	// fraction weights the gap to the next one.
	rank := p * float64(len(s)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return s[lo]
	}
	frac := rank - float64(lo)
	return s[lo] + (s[hi]-s[lo])*frac
}

// Median is the 50th percentile.
func Median(vals []float64) float64 { return Percentile(vals, 0.5) }

// Mean returns the arithmetic mean, or zero for an empty slice.
func Mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// StdDev returns the population standard deviation.
func StdDev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := Mean(vals)
	var sum float64
	for _, v := range vals {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(vals)))
}

// Min returns the smallest value, or zero for an empty slice.
func Min(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// Max returns the largest value, or zero for an empty slice.
func Max(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// Jitter is the mean absolute difference between consecutive samples.
//
// This is the IPDV definition from RFC 3393, which is what network
// people mean by jitter. It is not the standard deviation of the
// samples: a link that alternates 10 ms and 50 ms every packet has the
// same standard deviation as one that drifts slowly between them, but
// far worse jitter.
//
// Order matters, so the input must be in collection order.
func Jitter(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(vals); i++ {
		d := math.Abs(vals[i] - vals[i-1])
		// A single NaN/Inf in the trace must not poison the whole
		// summary; jitter is a mean, so one bad delta can be skipped.
		if d != d || d > 1e12 {
			continue
		}
		sum += d
	}
	return sum / float64(len(vals)-1)
}
