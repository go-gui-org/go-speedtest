package probe

import (
	"context"
	"testing"
)

// TestSimulatedTracePinsPeostaToChicago pins the demo's geography: the
// labels read "Peosta, Iowa" against a Chicago edge, so the trace must
// carry both ends. resolveLocations only knows country centroids for
// the client, which would drop the pin on Kansas; runSimulated owns
// its own coordinates and must override them.
func TestSimulatedTracePinsPeostaToChicago(t *testing.T) {
	cfg := Config{
		BaseURL:        "http://127.0.0.1:1",
		Simulate:       true,
		SimSpeed:       0.02,
		LatencySamples: 1,
	}
	evs := collect(t, New(cfg).Run(context.Background()))

	var tr *Trace
	for i := range evs {
		if evs[i].Kind == EventTrace && evs[i].Trace != nil {
			tr = evs[i].Trace
			break
		}
	}
	if tr == nil {
		t.Fatal("simulated run emitted no trace event")
	}
	if tr.Colo != "ORD" || !tr.ColoKnown || tr.ColoCity != "Chicago" {
		t.Errorf("colo = %q known=%v city=%q, want ORD/Chicago",
			tr.Colo, tr.ColoKnown, tr.ColoCity)
	}
	if !tr.ClientKnown {
		t.Fatal("client location not marked known")
	}
	// Literals, not simClientLat/simClientLng — comparing the code
	// against the constant it reads would pass for any value those
	// constants held, including the US centroid this exists to reject.
	const peostaLat, peostaLng = 42.4497, -90.8543
	if tr.ClientLat != peostaLat || tr.ClientLng != peostaLng {
		t.Errorf("client = (%v, %v), want Peosta (%v, %v)",
			tr.ClientLat, tr.ClientLng, peostaLat, peostaLng)
	}
}
