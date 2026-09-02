package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// collect drains an event stream into a slice, failing the test if the
// run does not finish promptly.
func collect(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var evs []Event
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return evs
			}
			evs = append(evs, ev)
		case <-deadline:
			t.Fatal("run did not finish within 30s")
		}
	}
}

// lastKind returns the kind of the final event, which is the run's
// verdict.
func lastKind(evs []Event) EventKind {
	if len(evs) == 0 {
		return EventError
	}
	return evs[len(evs)-1].Kind
}

func countKind(evs []Event, k EventKind) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == k {
			n++
		}
	}
	return n
}

func TestRunAgainstFakeEdge(t *testing.T) {
	edge := newFakeEdge(t)
	evs := collect(t, New(edge.config()).Run(context.Background()))

	if got := lastKind(evs); got != EventDone {
		t.Fatalf("last event kind = %v, want EventDone; events: %d", got, len(evs))
	}
	res := evs[len(evs)-1].Res
	if res == nil {
		t.Fatal("EventDone carried no Result")
	}

	// Trace parsed and resolved.
	if res.Trace.Colo != "SEA" {
		t.Errorf("colo = %q, want SEA", res.Trace.Colo)
	}
	if !res.Trace.ColoKnown || res.Trace.ColoCity != "Seattle" {
		t.Errorf("colo not resolved: %+v", res.Trace)
	}
	if !res.Trace.ClientKnown {
		t.Errorf("client location not resolved from loc=%q", res.Trace.Loc)
	}
	if res.Trace.IP != "203.0.113.7" {
		t.Errorf("ip = %q", res.Trace.IP)
	}

	// Every latency sample landed.
	if len(res.RTTs) != 3 {
		t.Errorf("RTT samples = %d, want 3", len(res.RTTs))
	}
	if n := countKind(evs, EventRTT); n != 3 {
		t.Errorf("EventRTT count = %d, want 3", n)
	}

	// Both directions transferred what was asked for.
	if res.DownBytes != 150_000 {
		t.Errorf("DownBytes = %d, want 150000", res.DownBytes)
	}
	if res.UpBytes != 50_000 {
		t.Errorf("UpBytes = %d, want 50000", res.UpBytes)
	}
	if edge.upBytes.Load() != 50_000 {
		t.Errorf("server received %d upload bytes, want 50000", edge.upBytes.Load())
	}
	if edge.downCalls.Load() != 2+3 { // two stages plus three latency probes
		t.Errorf("__down calls = %d, want 5", edge.downCalls.Load())
	}

	// One stage event per payload.
	if n := countKind(evs, EventStage); n != 3 {
		t.Errorf("EventStage count = %d, want 3", n)
	}

	if res.Simulated {
		t.Error("real run reported Simulated")
	}
	if res.Duration <= 0 {
		t.Error("Duration not recorded")
	}
}

func TestPhaseOrder(t *testing.T) {
	edge := newFakeEdge(t)
	evs := collect(t, New(edge.config()).Run(context.Background()))

	var phases []Phase
	for _, ev := range evs {
		if ev.Kind == EventPhase {
			phases = append(phases, ev.Phase)
		}
	}
	want := []Phase{PhaseTrace, PhaseLatency, PhaseDownload, PhaseUpload, PhaseDone}
	if len(phases) != len(want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
	for i := range want {
		if phases[i] != want[i] {
			t.Fatalf("phases = %v, want %v", phases, want)
		}
	}
}

func TestTraceFailureStopsRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusServiceUnavailable)
		}))
	defer srv.Close()

	evs := collect(t, New(Config{BaseURL: srv.URL}).Run(context.Background()))

	if countKind(evs, EventError) != 1 {
		t.Fatalf("want one EventError, got events %d", len(evs))
	}
	if got := evs[len(evs)-1].Phase; got != PhaseError {
		t.Errorf("final phase = %v, want PhaseError", got)
	}
	if countKind(evs, EventDone) != 0 {
		t.Error("failed run emitted EventDone")
	}
}

func TestCancellationEndsRun(t *testing.T) {
	edge := newFakeEdge(t)
	cfg := edge.config()
	cfg.LatencySamples = 500 // long enough that cancel lands mid-run

	ctx, cancel := context.WithCancel(context.Background())
	ch := New(cfg).Run(ctx)

	// Drain in the background so the engine is never blocked on a full
	// buffer; cancel once the run is clearly under way.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled run did not stop")
	}
}

func TestLadderRepeatsLargestUntilTimeIsUp(t *testing.T) {
	// The ladder must keep sending after its stages run out, or a fast
	// link is measured for a fraction of a second.
	var sent int64
	start := time.Now()
	var got []int64
	for size := range ladder([]int64{10, 20, 30}, 40*time.Millisecond, 1<<40, start, &sent) {
		got = append(got, size)
		sent += size
		// Stand in for the time a real transfer takes.
		time.Sleep(5 * time.Millisecond)
	}

	if len(got) < 4 {
		t.Fatalf("ladder yielded %d payloads, want the stages plus repeats", len(got))
	}
	for i, want := range []int64{10, 20, 30} {
		if got[i] != want {
			t.Errorf("payload %d = %d, want %d", i, got[i], want)
		}
	}
	for i, size := range got[3:] {
		if size != 30 {
			t.Errorf("repeat %d = %d, want the largest stage 30", i, size)
		}
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("ladder stopped after %v, before the minimum duration", elapsed)
	}
}

func TestLadderRepeatsAreBounded(t *testing.T) {
	// An endpoint that answers instantly must not spin the ladder for
	// the whole run timeout.
	var sent int64
	count := 0
	for range ladder([]int64{1}, time.Hour, 1<<40, time.Now(), &sent) {
		count++
	}
	if count != 1+maxLadderRepeats {
		t.Errorf("ladder yielded %d payloads, want %d", count, 1+maxLadderRepeats)
	}
}

func TestLadderStopsAtByteCap(t *testing.T) {
	// Without a cap, a gigabit link would spend a gigabyte reaching the
	// minimum duration.
	var sent int64
	start := time.Now()
	var count int
	for size := range ladder([]int64{100}, time.Hour, 450, start, &sent) {
		sent += size
		count++
		if count > 100 {
			t.Fatal("byte cap did not stop the ladder")
		}
	}
	if sent < 450 || sent > 550 {
		t.Errorf("sent %d bytes, want the ladder to stop just past the 450 cap", sent)
	}
}

func TestLadderWithNoStagesYieldsNothing(t *testing.T) {
	var sent int64
	for range ladder(nil, time.Hour, 1<<40, time.Now(), &sent) {
		t.Fatal("empty ladder yielded a payload")
	}
}

func TestPartialDownloadDoesNotKillTheRun(t *testing.T) {
	// A download that fails after producing readings must not discard
	// the upload phase: the two measure different things.
	edge := newFakeEdge(t)
	edge.failDownAfter.Store(4) // three latency probes and one payload, then fail

	cfg := edge.config()
	evs := collect(t, New(cfg).Run(context.Background()))

	if got := lastKind(evs); got != EventDone {
		t.Fatalf("last event = %v, want EventDone; a partial download ended the run", got)
	}
	if countKind(evs, EventWarning) != 1 {
		t.Errorf("want one EventWarning, got %d", countKind(evs, EventWarning))
	}
	res := evs[len(evs)-1].Res
	if res.UpBytes == 0 {
		t.Error("upload phase did not run after the download problem")
	}
}

func TestRateLimitIsNamed(t *testing.T) {
	// 429 says nothing about the link. The message must say so rather
	// than read as a network failure.
	edge := newFakeEdge(t)
	edge.downStatus.Store(http.StatusTooManyRequests)

	evs := collect(t, New(edge.config()).Run(context.Background()))
	var got error
	for _, ev := range evs {
		if ev.Kind == EventError {
			got = ev.Err
		}
	}
	if !errors.Is(got, ErrRateLimited) {
		t.Fatalf("error = %v, want ErrRateLimited", got)
	}
}

func TestSimulatedRunNeedsNoNetwork(t *testing.T) {
	// Point at an address nothing listens on: if the simulator touched
	// the network the run would fail rather than complete.
	cfg := Config{
		BaseURL:        "http://127.0.0.1:1",
		Simulate:       true,
		SimSpeed:       0.15,
		LatencySamples: 5,
	}
	evs := collect(t, New(cfg).Run(context.Background()))

	if got := lastKind(evs); got != EventDone {
		t.Fatalf("last event kind = %v, want EventDone", got)
	}
	res := evs[len(evs)-1].Res
	if !res.Simulated {
		t.Error("simulated run did not set Simulated")
	}
	if len(res.RTTs) != 5 {
		t.Errorf("RTT samples = %d, want 5", len(res.RTTs))
	}
	if res.Duration < 3*time.Second {
		t.Errorf("simulated run lasted %v; a demo that flashes past is not a demo", res.Duration)
	}
	if res.DownMbps <= 0 || res.UpMbps <= 0 {
		t.Errorf("throughput not produced: down=%v up=%v", res.DownMbps, res.UpMbps)
	}
	if res.DownMbps <= res.UpMbps {
		t.Errorf("demo should look asymmetric: down=%v up=%v", res.DownMbps, res.UpMbps)
	}
	if n := countKind(evs, EventRate); n < 20 {
		t.Errorf("EventRate count = %d, want the chart to have data", n)
	}
}

func TestUnknownColoDegradesQuietly(t *testing.T) {
	edge := newFakeEdge(t)
	edge.colo = "ZZZ" // not in the embedded table
	edge.loc = "ZZ"

	evs := collect(t, New(edge.config()).Run(context.Background()))
	if got := lastKind(evs); got != EventDone {
		t.Fatalf("unknown colo broke the run: last kind = %v", got)
	}
	tr := evs[len(evs)-1].Res.Trace
	if tr.ColoKnown || tr.ClientKnown {
		t.Errorf("unknown codes should stay unresolved: %+v", tr)
	}
	if tr.Colo != "ZZZ" {
		t.Errorf("colo code lost: %q", tr.Colo)
	}
}
