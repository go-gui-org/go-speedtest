package app

import (
	"math"
	"testing"
	"time"

	"github.com/go-gui-org/go-charts/series"
	"github.com/go-gui-org/go-map/projection"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

func TestGaugeScaleClimbsAndNeverClips(t *testing.T) {
	cases := []struct {
		peak float64
		want float64
	}{
		{0, 25},
		{20, 25},
		{23, 50},  // 23 > 25*0.9, so step up
		{47, 100}, // 47 > 50*0.9
		{95, 250}, // 95 > 100*0.9
		{940, 2500},
		{9000, 5000},
	}
	for _, c := range cases {
		s := &State{PeakDown: c.peak}
		if got := gaugeScale(s); got != c.want {
			t.Errorf("gaugeScale(peak=%v) = %v, want %v", c.peak, got, c.want)
		}
	}
}

func TestGaugeScaleAlwaysLeavesHeadroom(t *testing.T) {
	// The needle must never sit at or past the end of the arc, or it
	// reads as "off the scale" for a perfectly normal link.
	for peak := 1.0; peak < 3000; peak *= 1.13 {
		s := &State{PeakDown: peak}
		if scale := gaugeScale(s); scale <= peak {
			t.Fatalf("peak %v exceeds scale %v", peak, scale)
		}
	}
}

func TestGaugeValueFollowsPhase(t *testing.T) {
	s := &State{Phase: probe.PhaseUpload, Live: 42}
	if v, c := gaugeValue(s); v != 42 || c != colorUp {
		t.Errorf("upload phase = (%v, %v), want (42, colorUp)", v, c)
	}

	s = &State{Phase: probe.PhaseDownload, Live: 300}
	if v, c := gaugeValue(s); v != 300 || c != colorDown {
		t.Errorf("download phase = (%v, %v)", v, c)
	}

	// A finished run shows its headline figure, not the last live
	// reading, which is zero by then.
	s = &State{Phase: probe.PhaseDone, Result: &probe.Result{DownMbps: 512}}
	if v, _ := gaugeValue(s); v != 512 {
		t.Errorf("finished run = %v, want 512", v)
	}
}

func TestStatusLineStates(t *testing.T) {
	idle := &State{Phase: probe.PhaseIdle}
	if got := statusLine(idle); got != "Ready" {
		t.Errorf("idle = %q", got)
	}

	failed := &State{Phase: probe.PhaseError, Err: errTest{}}
	if got := statusLine(failed); got != "failed: boom" {
		t.Errorf("failed = %q", got)
	}

	// A simulated result must say so. A demo that reads as a real
	// measurement is the one genuinely misleading state this app has.
	sim := &State{Phase: probe.PhaseDone, Result: &probe.Result{Simulated: true}}
	if got := statusLine(sim); got != "Complete (simulated, no network used)" {
		t.Errorf("simulated = %q", got)
	}

	real := &State{Phase: probe.PhaseDone, Result: &probe.Result{}}
	if got := statusLine(real); got != "Complete" {
		t.Errorf("real = %q", got)
	}
}

func TestTraceSuffix(t *testing.T) {
	none := &State{}
	if got := traceSuffix(none); got != "" {
		t.Errorf("no trace = %q", got)
	}

	known := &State{Trace: &probe.Trace{Colo: "SEA", ColoCity: "Seattle", ColoKnown: true}}
	if got := traceSuffix(known); got != "  ·  Seattle (SEA)" {
		t.Errorf("known = %q", got)
	}

	// An unresolved code still shows: the code itself is useful.
	unknown := &State{Trace: &probe.Trace{Colo: "ZZZ"}}
	if got := traceSuffix(unknown); got != "  ·  ZZZ" {
		t.Errorf("unknown = %q", got)
	}
}

func TestLatestX(t *testing.T) {
	down := series.NewXY(series.XYCfg{Points: []series.Point{{X: 1}, {X: 4.5}}})
	up := series.NewXY(series.XYCfg{Points: []series.Point{{X: 7.25}}})

	if got := latestX(down, up); got != 7.25 {
		t.Errorf("latestX = %v, want 7.25", got)
	}
	if got := latestX(); got != 0 {
		t.Errorf("latestX() = %v, want 0", got)
	}
	if got := latestX(series.XY{}); got != 0 {
		t.Errorf("latestX(empty) = %v, want 0", got)
	}
}

func TestLiveWindowGrowsThenStops(t *testing.T) {
	// Before the floor: a minimum width, so the first two readings do
	// not stretch across the whole panel.
	if got := liveWindow(0); got != minLiveSeconds {
		t.Errorf("liveWindow(0) = %v, want %v", got, minLiveSeconds)
	}

	// While growing: the window matches the run, which keeps the left
	// edge at zero and the curve filling the panel.
	if got := liveWindow(9); got != 9 {
		t.Errorf("liveWindow(9) = %v, want 9", got)
	}

	// After the cap: fixed width, so the window slides instead.
	if got := liveWindow(40); got != liveSeconds {
		t.Errorf("liveWindow(40) = %v, want %v", got, liveSeconds)
	}
}

func TestBoundsOfCoversEveryPoint(t *testing.T) {
	pts := []projection.LatLng{
		{Lat: 47.45, Lng: -122.31},
		{Lat: 37.09, Lng: -95.71},
	}
	b := boundsOf(pts)
	if b.NE.Lat != 47.45 || b.SW.Lat != 37.09 {
		t.Errorf("latitude bounds wrong: %+v", b)
	}
	if b.NE.Lng != -95.71 || b.SW.Lng != -122.31 {
		t.Errorf("longitude bounds wrong: %+v", b)
	}
}

func TestGreatCircleEndsWhereItShould(t *testing.T) {
	seattle := projection.LatLng{Lat: 47.45, Lng: -122.31}
	london := projection.LatLng{Lat: 51.47, Lng: -0.45}

	pts := greatCircle(seattle, london, 32)
	if len(pts) != 33 {
		t.Fatalf("got %d points, want 33", len(pts))
	}
	assertNear(t, pts[0], seattle, "start")
	assertNear(t, pts[len(pts)-1], london, "end")

	// The arc must bend north of the straight interpolation: that is
	// the whole reason for drawing it rather than a line.
	mid := pts[16]
	flatMid := (seattle.Lat + london.Lat) / 2
	if mid.Lat <= flatMid {
		t.Errorf("arc midpoint lat %v does not bow north of %v", mid.Lat, flatMid)
	}
}

func TestGreatCircleDegenerate(t *testing.T) {
	p := projection.LatLng{Lat: 10, Lng: 20}
	pts := greatCircle(p, p, 16)
	if len(pts) != 2 {
		t.Fatalf("identical endpoints gave %d points, want 2", len(pts))
	}
}

func TestResetClearsPreviousRun(t *testing.T) {
	s := New(true, time.Minute, nil)
	s.RTTms = append(s.RTTms, 12, 13)
	s.Down.Append(series.Point{X: 1, Y: 100})
	s.Trace = &probe.Trace{Colo: "SEA"}
	s.Result = &probe.Result{DownMbps: 9}
	s.Err = errTest{}
	s.PeakDown = 400
	s.mapFitted = true

	s.reset()

	if len(s.RTTms) != 0 {
		t.Errorf("RTTms not cleared: %v", s.RTTms)
	}
	if n := len(s.Down.Snapshot().Points); n != 0 {
		t.Errorf("download series not cleared: %d points", n)
	}
	if s.Trace != nil || s.Result != nil || s.Err != nil {
		t.Error("previous run's results survived reset")
	}
	if s.PeakDown != 0 {
		t.Errorf("PeakDown = %v", s.PeakDown)
	}
	if s.mapFitted {
		t.Error("mapFitted survived reset; the map would not reframe")
	}
}

func TestCancelLifecycle(t *testing.T) {
	s := New(true, time.Minute, nil)
	if s.Running() {
		t.Fatal("new state reports a run in progress")
	}

	stopped := false
	s.setCancel(func() { stopped = true })
	if !s.Running() {
		t.Fatal("setCancel did not mark the run running")
	}

	// A second Start must cancel the first, or the old run keeps
	// writing into the series behind the new one.
	s.setCancel(func() {})
	if !stopped {
		t.Error("replacing the canceller did not cancel the previous run")
	}

	if !s.clearCancel() {
		t.Error("clearCancel reported nothing to cancel")
	}
	if s.Running() {
		t.Error("still running after clearCancel")
	}
	if s.clearCancel() {
		t.Error("clearCancel reported a second cancellation")
	}
}

func TestItoa(t *testing.T) {
	for _, n := range []int{0, 1, 9, 10, 4207} {
		if got, want := itoa(n), decimal(n); got != want {
			t.Errorf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestHardenedAppHelpers(t *testing.T) {
	// itoa must not return "" on negative.
	if got := itoa(-5); got != "-5" {
		t.Errorf("itoa(-5) = %q, want -5", got)
	}
	if got := liveWindow(math.NaN()); got != minLiveSeconds {
		t.Errorf("liveWindow(NaN) = %v, want %v", got, minLiveSeconds)
	}
	if got := liveWindow(-3); got != minLiveSeconds {
		t.Errorf("liveWindow(-3) = %v, want %v", got, minLiveSeconds)
	}
	// gaugeScale must not propagate NaN into zone thresholds.
	s := &State{PeakDown: math.NaN()}
	if got := gaugeScale(s); got != 25 {
		t.Errorf("gaugeScale(NaN) = %v, want 25", got)
	}
	// boundsOf on empty must not panic.
	if got := boundsOf(nil); got.NE.Lat != 0 || got.SW.Lat != 0 {
		t.Errorf("boundsOf(nil) = %+v, want zero", got)
	}
	pts := greatCircle(projection.LatLng{Lat: 0, Lng: 0}, projection.LatLng{Lat: 10, Lng: 10}, 0)
	if len(pts) != 2 {
		t.Errorf("greatCircle segments=0 gave %d points, want 2", len(pts))
	}
	pts = greatCircle(projection.LatLng{Lat: 0, Lng: 0}, projection.LatLng{Lat: math.NaN(), Lng: 0}, 8)
	if len(pts) != 2 {
		t.Errorf("greatCircle NaN gave %d points, want 2", len(pts))
	}
	if got := latestX(series.NewXY(series.XYCfg{Points: []series.Point{{X: math.NaN()}}})); got != 0 {
		t.Errorf("latestX(NaN) = %v, want 0", got)
	}
}

// decimal is a deliberately dumb reference implementation.
func decimal(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// assertNear compares two positions within a tenth of a degree, which
// is far tighter than the arc sampling error.
func assertNear(t *testing.T, got, want projection.LatLng, what string) {
	t.Helper()
	if math.Abs(got.Lat-want.Lat) > 0.1 || math.Abs(got.Lng-want.Lng) > 0.1 {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

// errTest is a fixed error for status-line assertions.
type errTest struct{}

func (errTest) Error() string { return "boom" }
