// Package format turns measurements into the strings the UI and the CLI
// report both show. One place, so a number never reads differently in
// two panels.
package format

import (
	"fmt"
	"time"
)

// Mbps formats a throughput reading. The precision steps down as the
// number grows: 8.42 Mbps is a meaningful distinction, 942.7 Mbps is
// not.
func Mbps(v float64) string {
	// Non-finite readings come from degenerate timing (zero duration)
	// and must not reach the UI as "NaN Mbps".
	if v != v || v <= 0 || v > 1e12 {
		return "0"
	}
	switch {
	case v < 10:
		return fmt.Sprintf("%.2f", v)
	case v < 100:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

// Millis formats a duration as milliseconds, the unit every latency
// figure in this app uses.
func Millis(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	if ms < 10 {
		return fmt.Sprintf("%.1f", ms)
	}
	return fmt.Sprintf("%.0f", ms)
}

// MillisF formats a latency already held as a float in milliseconds.
func MillisF(ms float64) string {
	if ms != ms || ms < 0 {
		return "0"
	}
	if ms < 10 {
		return fmt.Sprintf("%.1f", ms)
	}
	return fmt.Sprintf("%.0f", ms)
}

// Bytes formats a transfer total in base-ten units, matching how link
// speeds are quoted.
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
		if exp >= 5 {
			break
		}
	}
	if exp >= len("kMGTPE") {
		exp = len("kMGTPE") - 1
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
