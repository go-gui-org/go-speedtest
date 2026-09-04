package app

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/go-gui-org/go-charts/series"
	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-map/projection"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

func TestGaugeRangeSwitchesAtGigabit(t *testing.T) {
	// The base range counts in Mbps and reads the value straight.
	top, div, unit, _ := gaugeRange(&State{})
	if top != gaugeMax || div != 1 || unit != "Mbps" {
		t.Errorf("base range = (%v, %v, %q)", top, div, unit)
	}

	// The gigabit range counts in Gbps, so the divisor must turn a
	// reading in Mbps into a number the dial's own scale accepts.
	top, div, unit, _ = gaugeRange(&State{HighRange: true})
	if top != gaugeMaxGbps || unit != "Gbps" {
		t.Errorf("high range = (%v, %q)", top, unit)
	}
	if got := 2500 / div; got != 2.5 {
		t.Errorf("2500 Mbps on the gigabit dial = %v, want 2.5", got)
	}
	if gaugeMax/div > top {
		t.Error("the switching point must fall on the gigabit dial")
	}
}

func TestGaugeScaleIsFixed(t *testing.T) {
	// The dial is fixed at 0..gaugeMax so a needle angle means the same
	// speed in every run. The zone thresholds must stay inside it, or
	// go-charts rejects the config.
	if gaugeMax <= 0 {
		t.Fatalf("gaugeMax = %v", gaugeMax)
	}
	if gaugeMax*0.6 >= gaugeMax {
		t.Error("zone thresholds must ascend below the maximum")
	}
}

func TestGaugeValueFollowsPhase(t *testing.T) {
	s := &State{Phase: probe.PhaseUpload, LiveUp: 42}
	if v, c := gaugeValue(s); v != 42 || c != colorUp {
		t.Errorf("upload phase = (%v, %v), want (42, colorUp)", v, c)
	}

	s = &State{Phase: probe.PhaseDownload, LiveDown: 300}
	if v, c := gaugeValue(s); v != 300 || c != colorDown {
		t.Errorf("download phase = (%v, %v)", v, c)
	}

	// A finished run shows its headline figure, not the last live
	// reading, which the filter leaves short of the mean.
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
	s.RTTIdle.add(0.1, 12)
	s.RTTIdle.add(0.2, 13)
	s.RTTDown.add(3, 90)
	s.RTTDown.add(3.5, 95)
	s.Down.Append(series.Point{X: 1, Y: 100})
	s.Trace = &probe.Trace{Colo: "SEA"}
	s.Result = &probe.Result{DownMbps: 9}
	s.Err = errTest{}
	s.LiveDown = 400
	s.mapFitted = true

	s.reset()

	if s.RTTIdle.len() != 0 || len(s.RTTIdle.Pts) != 0 {
		t.Errorf("idle samples not cleared: %v", s.RTTIdle.Vals)
	}
	if s.RTTDown.len() != 0 || len(s.RTTDown.Pts) != 0 {
		t.Errorf("loaded samples not cleared: %v", s.RTTDown.Vals)
	}
	if n := len(s.Down.Snapshot().Points); n != 0 {
		t.Errorf("download series not cleared: %d points", n)
	}
	if s.Trace != nil || s.Result != nil || s.Err != nil {
		t.Error("previous run's results survived reset")
	}
	if s.LiveDown != 0 {
		t.Errorf("LiveDown = %v", s.LiveDown)
	}
	if s.mapFitted {
		t.Error("mapFitted survived reset; the map would not reframe")
	}
}

func TestRunLifecycle(t *testing.T) {
	s := New(true, time.Minute, nil)
	if s.Running() {
		t.Fatal("new state reports a run in progress")
	}

	ctx, gen := s.beginRun()
	if !s.Running() {
		t.Fatal("beginRun did not mark the run running")
	}
	if !s.isCurrentRun(gen) {
		t.Fatal("the run beginRun started is not the current one")
	}

	// A second Start must cancel the first, or the old run keeps
	// writing into the series behind the new one.
	_, gen2 := s.beginRun()
	if ctx.Err() == nil {
		t.Error("starting a second run did not cancel the first")
	}
	if s.isCurrentRun(gen) {
		t.Error("the replaced run is still current; its events would land")
	}
	if !s.isCurrentRun(gen2) {
		t.Error("the new run is not current")
	}

	// A late finish from the run that was replaced must not clear the
	// canceller of the run that replaced it.
	s.finishRun(gen)
	if !s.Running() {
		t.Error("a retired run's finish stopped the current one")
	}

	if !s.endRun() {
		t.Error("endRun reported nothing to cancel")
	}
	if s.Running() {
		t.Error("still running after endRun")
	}
	if s.isCurrentRun(gen2) {
		t.Error("a stopped run is still current; its buffered events would land")
	}
	if s.endRun() {
		t.Error("endRun reported a second cancellation")
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
	// A NaN/Inf/negative/huge reading must not poison the dial's filter.
	if got := smoothLive(100, math.NaN()); got != 100 {
		t.Errorf("smoothLive(100, NaN) = %v, want 100", got)
	}
	if got := smoothLive(100, math.Inf(1)); got != 100 {
		t.Errorf("smoothLive(100, +Inf) = %v, want 100", got)
	}
	if got := smoothLive(100, -5); got != 100 {
		t.Errorf("smoothLive(100, -5) = %v, want 100", got)
	}
	if got := smoothLive(100, 2e6); got != 100 {
		t.Errorf("smoothLive(100, 2e6) = %v, want 100", got)
	}
	// bounds must skip Inf, not just NaN, otherwise boxYAxis gets Inf domain.
	if lo, hi := bounds([]float64{math.Inf(1), 5, 10, math.Inf(-1)}); lo != 5 || hi != 10 {
		t.Errorf("bounds(Inf,5,10,Inf) = %v,%v want 5,10", lo, hi)
	}
	if lo, hi := bounds([]float64{math.Inf(1), math.Inf(-1)}); lo != 0 || hi != 1 {
		t.Errorf("bounds(all Inf) = %v,%v want 0,1", lo, hi)
	}
	// mbpsTick near-integer due to floating error must still format as integer.
	if got := mbpsTick(99.9999999998); got != "100" {
		t.Errorf("mbpsTick(99.9999999998) = %q, want 100", got)
	}
	if got := mbpsTick(100.0000000002); got != "100" {
		t.Errorf("mbpsTick(100.0000000002) = %q, want 100", got)
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

func TestConnectionFieldsFormat(t *testing.T) {
	// The family comes from the address itself: a colon can only
	// appear in an IPv6 literal.
	for ip, want := range map[string]string{
		"203.0.113.7":  "IPv4",
		"2606:4700::1": "IPv6",
		"":             "—",
	} {
		if got := ipFamily(ip); got != want {
			t.Errorf("ipFamily(%q) = %q, want %q", ip, got, want)
		}
	}

	// A datacenter shows its city and its code together; a code we
	// cannot name still shows, because the code alone is useful.
	full := &probe.Trace{Colo: "SEA", ColoCity: "Seattle"}
	if got := coloPlace(full); got != "Seattle (SEA)" {
		t.Errorf("coloPlace = %q", got)
	}
	if got := coloPlace(&probe.Trace{Colo: "ZZZ"}); got != "ZZZ" {
		t.Errorf("coloPlace(unknown) = %q", got)
	}
	if got := coloPlace(&probe.Trace{}); got != "—" {
		t.Errorf("coloPlace(empty) = %q", got)
	}

	// The network line must survive a meta call that never answered,
	// which is the common case on a locked-down network.
	if got := networkName(&probe.Trace{}); got != "—" {
		t.Errorf("networkName(empty) = %q", got)
	}
	if got := networkName(&probe.Trace{ASN: 64512}); got != "AS64512" {
		t.Errorf("networkName(asn only) = %q", got)
	}
	if got := networkName(&probe.Trace{ASOrg: "EXAMPLE NET"}); got != "EXAMPLE NET" {
		t.Errorf("networkName(org only) = %q", got)
	}
	want := "EXAMPLE NET  (AS64512)"
	if got := networkName(&probe.Trace{ASOrg: "EXAMPLE NET", ASN: 64512}); got != want {
		t.Errorf("networkName = %q, want %q", got, want)
	}
}

func TestMbpsTickKeepsOneScale(t *testing.T) {
	// Whole ticks must not gain a decimal the neighbouring tick lacks:
	// that is what made an axis read 140, 120, 100, 80.0, 60.0.
	for v, want := range map[float64]string{
		0: "0", 20: "20", 140: "140", 2.5: "2.5",
	} {
		if got := mbpsTick(v); got != want {
			t.Errorf("mbpsTick(%v) = %q, want %q", v, got, want)
		}
	}
}

func TestSelectProvider(t *testing.T) {
	s := New(false, time.Minute, nil)
	if got := s.Provider().Name; got != "Cloudflare" {
		t.Fatalf("default provider = %q, want Cloudflare", got)
	}

	if err := s.SelectProvider(probe.Selection{Provider: "demo"}); err != nil {
		t.Fatalf("SelectProvider(demo) = %v", err)
	}
	if !s.Provider().Simulate {
		t.Error("demo provider does not simulate")
	}

	// A custom entry with no URL is refused, and the refusal must not
	// move the selection: a failed switch leaves the app on the
	// provider it was already using.
	before := s.ProviderIdx
	if err := s.SelectProvider(probe.Selection{Provider: "custom"}); err == nil {
		t.Error("SelectProvider(custom, \"\") = nil, want an error")
	}
	if s.ProviderIdx != before {
		t.Errorf("failed switch moved the selection to %d", s.ProviderIdx)
	}

	if err := s.SelectProvider(probe.Selection{
		Provider: "custom", CustomURL: "https://h.example",
	}); err != nil {
		t.Fatalf("SelectProvider(custom, url) = %v", err)
	}
	cfg := s.Provider().Apply(probe.Config{}, s.CustomURL, s.ServerIdx)
	if cfg.BaseURL != "https://h.example" || cfg.Simulate {
		t.Errorf("custom applied to %+v", cfg)
	}

	if err := s.SelectProvider(probe.Selection{Provider: "nonesuch"}); err == nil {
		t.Error("SelectProvider(nonesuch) = nil, want an error")
	}
}

func TestSelectLibreSpeedServer(t *testing.T) {
	s := New(false, time.Minute, nil)
	if err := s.SelectProvider(probe.Selection{
		Provider: "libre", Server: "tokyo",
	}); err != nil {
		t.Fatalf("SelectProvider(libre, tokyo) = %v", err)
	}
	if !strings.Contains(strings.ToLower(s.Server().Name), "tokyo") {
		t.Errorf("selected server %q, want a Tokyo one", s.Server().Name)
	}

	// Switching to a provider with no list must leave Server() harmless
	// rather than indexing the old list.
	s.ProviderIdx, s.ServerIdx = probe.DemoProvider, 5
	if got := s.Server(); got.Name != "" {
		t.Errorf("Server() on a listless provider = %+v", got)
	}
}

func TestNewDemoSelectsTheOfflineProvider(t *testing.T) {
	if got := New(true, time.Minute, nil).Provider().Name; got != "Demo (offline)" {
		t.Errorf("New(demo) provider = %q, want Demo (offline)", got)
	}
}

// TestPumpIgnoresARetiredRun reproduces the chart filling backwards.
//
// Starting a second test replaces the first, but the first engine's
// channel still holds events, and its pump goroutine drains them after
// the swap. Those events carry the old run's clock — eighteen seconds
// in, where the new run is at zero — so appending them put points to
// the right of everything the new run had drawn.
func TestPumpIgnoresARetiredRun(t *testing.T) {
	s := New(true, time.Minute, nil)
	w := &gui.Window{}

	_, gen := s.beginRun()
	// What pressing Start a second time does.
	_, _ = s.beginRun()
	s.reset()

	events := make(chan probe.Event, 4)
	events <- probe.Event{Kind: probe.EventRate, Phase: probe.PhaseDownload, Mbps: 100}
	events <- probe.Event{Kind: probe.EventRate, Phase: probe.PhaseUpload, Mbps: 50}
	close(events)
	// The old run's clock: eighteen seconds ahead of the new one.
	pump(w, s, events, time.Now().Add(-18*time.Second), gen)

	if n := len(s.Down.Snapshot().Points); n != 0 {
		t.Errorf("a retired run appended %d points to the new chart", n)
	}
	if n := len(s.Up.Snapshot().Points); n != 0 {
		t.Errorf("a retired run appended %d upload points", n)
	}
}

// The same pump, still current, must publish normally.
func TestPumpAppendsForTheCurrentRun(t *testing.T) {
	s := New(true, time.Minute, nil)
	w := &gui.Window{}

	_, gen := s.beginRun()
	s.reset()

	events := make(chan probe.Event, 2)
	events <- probe.Event{Kind: probe.EventRate, Phase: probe.PhaseDownload, Mbps: 100}
	close(events)
	pump(w, s, events, time.Now(), gen)

	if n := len(s.Down.Snapshot().Points); n != 1 {
		t.Errorf("current run appended %d points, want 1", n)
	}
}

// A stopped run must go quiet too: Stop cancels, but the engine's
// buffered events are still behind it.
func TestPumpIgnoresAStoppedRun(t *testing.T) {
	s := New(true, time.Minute, nil)
	w := &gui.Window{}

	_, gen := s.beginRun()
	s.reset()
	s.endRun()

	events := make(chan probe.Event, 2)
	events <- probe.Event{Kind: probe.EventRate, Phase: probe.PhaseDownload, Mbps: 100}
	close(events)
	pump(w, s, events, time.Now(), gen)

	if n := len(s.Down.Snapshot().Points); n != 0 {
		t.Errorf("a stopped run appended %d points", n)
	}
}
