package format

import (
	"math"
	"testing"
	"time"
)

func TestMbpsPrecisionSteps(t *testing.T) {
	cases := map[float64]string{
		0:      "0",
		-3:     "0",
		8.417:  "8.42",
		42.36:  "42.4",
		942.71: "943",
	}
	for in, want := range cases {
		if got := Mbps(in); got != want {
			t.Errorf("Mbps(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestMillis(t *testing.T) {
	if got := Millis(2400 * time.Microsecond); got != "2.4" {
		t.Errorf("Millis(2.4ms) = %q", got)
	}
	if got := Millis(24 * time.Millisecond); got != "24" {
		t.Errorf("Millis(24ms) = %q", got)
	}
}

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		512:        "512 B",
		1_500:      "1.5 kB",
		25_000_000: "25.0 MB",
	}
	for in, want := range cases {
		if got := Bytes(in); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHardenedFormat(t *testing.T) {
	if got := Mbps(math.NaN()); got != "0" {
		t.Errorf("Mbps(NaN) = %q, want 0", got)
	}
	if got := Mbps(math.Inf(1)); got != "0" {
		t.Errorf("Mbps(Inf) = %q, want 0", got)
	}
	if got := MillisF(math.NaN()); got != "0" {
		t.Errorf("MillisF(NaN) = %q, want 0", got)
	}
	if got := MillisF(-1); got != "0" {
		t.Errorf("MillisF(-1) = %q, want 0", got)
	}
	if got := Bytes(-5); got != "0 B" {
		t.Errorf("Bytes(-5) = %q, want 0 B", got)
	}
	// Beyond the table: must not index out of range.
	if got := Bytes(1 << 62); got == "" {
		t.Error("Bytes(huge) returned empty string")
	}
}
