// Package app is the dashboard: window state, the view tree, and the
// bridge between the measurement engine and the screen.
//
// The split from internal/probe is deliberate. Nothing here measures
// anything, and nothing there knows a window exists.
package app

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/go-gui-org/go-charts/chart"
	"github.com/go-gui-org/go-charts/series"
	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-map/tile"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

// seriesCapacity is how many instantaneous readings the throughput
// chart keeps. At one reading per 50 ms this is a two-minute tail,
// longer than any run, so a finished run stays fully on screen.
const seriesCapacity = 2400

// State is the window's single typed slot.
//
// The event pump writes it under the window lock and the view reads it
// under the same lock, which is the pattern process_monitor uses. The
// two RealTimeSeries are the exception: they carry their own mutex, so
// the pump appends to them without taking the window lock at all.
type State struct {
	// Down and Up hold the live throughput readings. Both are plotted
	// against seconds since the run started, so the two curves lie on
	// one continuous timeline rather than overlapping at zero.
	Down *chart.RealTimeSeries
	Up   *chart.RealTimeSeries

	// Latency samples, one set per phase, each plotted against seconds
	// since the run began so the three lie on the same timeline as the
	// throughput charts. Kept apart because the comparison is the
	// measurement: latency at rest next to latency while the link is
	// busy is what says whether a call survives someone else starting
	// a download.
	RTTIdle rttPhase
	RTTDown rttPhase
	RTTUp   rttPhase

	Phase  probe.Phase
	Trace  *probe.Trace
	Result *probe.Result
	// Err is a fatal error: the run stopped. Warn is a phase that
	// ended early while the run carried on.
	Err     error
	Warn    error
	Started time.Time

	// LiveDown and LiveUp are the smoothed reading of each direction.
	// Both the dial and the stat text read them, so the two always
	// agree, and the download figure stays on screen through the
	// upload phase.
	LiveDown float64
	LiveUp   float64

	// HighRange latches once a reading passes the top of the dial's
	// base range, switching it to the gigabit scale. It latches rather
	// than following the current reading because a dial that changed
	// range whenever the rate crossed a gigabit would relabel itself
	// several times a second on a link sitting near the boundary.
	HighRange bool

	// Timeout bounds a run.
	Timeout time.Duration

	// Providers is the list the picker shows, and ProviderIdx is the
	// entry a new run uses. The list is held rather than fetched per
	// frame because the combobox takes a []string and rebuilding one
	// on every frame would allocate for nothing.
	Providers   []probe.Provider
	ProviderIdx int
	// CustomURL is the origin typed into the field the custom provider
	// reveals. Kept even while another provider is selected, so
	// switching away and back does not lose what was typed.
	CustomURL string
	// ProviderErr is what is wrong with CustomURL, shown under the
	// picker. Set when a run is refused, cleared on the next edit.
	ProviderErr error
	// ServerIdx is the entry a run uses from the selected provider's
	// server list, for a provider that publishes one. Held per app
	// rather than per provider because only one provider has servers;
	// the day a second one does, this becomes a map keyed by name.
	ServerIdx int

	// Tiles is the map's tile provider. Held here rather than built in
	// the view, so every frame reuses one cache and one user agent.
	Tiles tile.Source

	// Version increments whenever the charts have new data. Charts key
	// their transition animation off it, so bumping it only on real
	// change keeps idle frames from re-tweening.
	Version uint64

	// cancelMu guards cancel alone. The field is written from three
	// places — a button callback, OnInit, and the event pump — which do
	// not all hold the same lock, so it carries its own. Everything
	// else in State is guarded by the window lock.
	cancelMu sync.Mutex
	// cancel stops the run in progress. Nil when nothing is running.
	cancel context.CancelFunc
	// runGen counts runs, and is what tells a late event which run it
	// belongs to.
	//
	// Cancelling a run does not stop its pump goroutine at once: the
	// engine's channel is buffered, so events already in it are still
	// drained and published after the cancel returns. Those events
	// carry the old run's clock. Landing them in the new run's series
	// puts points at eighteen seconds next to points at zero, which is
	// what drew the chart filling backwards. Every write from a pump
	// is checked against this first, and a retired generation's writes
	// are dropped.
	runGen uint64
	// mapFitted guards the one-time map framing, so a redraw does not
	// yank a viewport the user has since panned.
	mapFitted bool
}

// New builds the initial state.
//
// demo selects the offline provider up front, which is what the -demo
// flag means: it is a starting selection, not a mode. The picker can
// move off it, and back to it, without restarting the app.
func New(demo bool, timeout time.Duration, tiles tile.Source) *State {
	idx := probe.DefaultProvider
	if demo {
		idx = probe.DemoProvider
	}
	return &State{
		Down:        chart.NewRealTimeSeries(chart.RealTimeSeriesCfg{Name: "Download", Color: colorDown, MaxLen: seriesCapacity}),
		Up:          chart.NewRealTimeSeries(chart.RealTimeSeriesCfg{Name: "Upload", Color: colorUp, MaxLen: seriesCapacity}),
		Timeout:     timeout,
		Tiles:       tiles,
		Phase:       probe.PhaseIdle,
		Providers:   probe.Providers(),
		ProviderIdx: idx,
	}
}

// SelectProvider points the app at a named provider before the window
// opens. It is how the command line reaches the same list the picker
// shows, so a headless run and a clicked one cannot disagree about what
// the names mean.
//
// A custom URL is checked here rather than at the first run: a bad one
// given on the command line should be a startup error, not a failure
// eight seconds into a test.
func (s *State) SelectProvider(sel probe.Selection) error {
	_, providerIdx, serverIdx, err := sel.Resolve()
	if err != nil {
		return err
	}
	s.ProviderIdx = providerIdx
	s.ServerIdx = serverIdx
	s.CustomURL = sel.CustomURL
	return nil
}

// Server is the entry a run started now would talk to, for a provider
// that publishes a server list. The zero value for one that does not.
//
// Bounds-checked for the same reason Provider is: ServerIdx survives a
// change of provider, so it can outlive the list it indexed.
func (s *State) Server() probe.Server {
	p := s.Provider()
	if len(p.Servers) == 0 {
		return probe.Server{}
	}
	return p.Servers[probe.ClampServer(p, s.ServerIdx)]
}

// Provider is the entry a run started now would use.
//
// Bounds-checked rather than indexed directly: ProviderIdx is written
// from a UI callback that carries a name looked up in a list, and a
// stale frame is a cheaper thing to survive than a panic.
func (s *State) Provider() probe.Provider {
	if s.ProviderIdx < 0 || s.ProviderIdx >= len(s.Providers) {
		return probe.Provider{}
	}
	return s.Providers[s.ProviderIdx]
}

// state is the typed accessor. Callbacks reach app state through it
// rather than repeating the assertion at every site.
func state(w *gui.Window) *State { return gui.State[State](w) }

// Running reports whether a run is in progress.
func (s *State) Running() bool {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.cancel != nil
}

// beginRun retires the run in progress and claims the next generation.
//
// The cancel and the generation bump happen under one lock, so a pump
// goroutine can never observe a state where its run has been replaced
// but its generation is still current.
func (s *State) beginRun() (context.Context, uint64) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	prev := s.cancel
	s.cancel = cancel
	s.runGen++
	gen := s.runGen
	s.cancelMu.Unlock()
	// Outside the lock: a cancel func runs arbitrary callbacks.
	if prev != nil {
		prev()
	}
	return ctx, gen
}

// endRun cancels the run in progress and retires its generation, so
// nothing still draining from it can write. Reports whether there was
// a run to cancel.
func (s *State) endRun() bool {
	s.cancelMu.Lock()
	c := s.cancel
	s.cancel = nil
	// Bumped even when there was no run: the counter only has to move
	// forward, and an unconditional bump is one less case to reason
	// about.
	s.runGen++
	s.cancelMu.Unlock()
	if c == nil {
		return false
	}
	c()
	return true
}

// finishRun marks a run that ended on its own as no longer running.
//
// Unlike endRun it does not retire the generation: the run's own last
// events — the result, the final phase — still have to land, and the
// engine closes the channel after them, so nothing else can follow.
func (s *State) finishRun(gen uint64) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.runGen != gen || s.cancel == nil {
		return
	}
	s.cancel = nil
}

// isCurrentRun reports whether gen is still the run the app is showing.
// Every write from a pump goroutine is gated on it.
func (s *State) isCurrentRun(gen uint64) bool {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.runGen == gen
}

// reset clears everything a previous run left behind. Called on the
// window thread before a new run starts.
func (s *State) reset() {
	s.Down.Clear()
	s.Up.Clear()
	s.RTTIdle.reset()
	s.RTTDown.reset()
	s.RTTUp.reset()
	s.Trace = nil
	s.Result = nil
	s.Err = nil
	s.Warn = nil
	s.LiveDown = 0
	s.LiveUp = 0
	s.HighRange = false
	s.mapFitted = false
	s.Version++
}

// liveSmoothing is the weight a new reading gets in the displayed rate.
//
// Rates arrive every 50ms (probe.rateInterval), and the raw figure
// swings by tens of Mbps between samples, so an unfiltered needle blurs
// and the text under it flickers. This is a one-pole low pass,
// alpha = 1-exp(-dt/tau) with tau = 1s: a step is most of the way there
// in about a second, which gives the needle the weight of an analog
// movement without hiding a real change in rate.
const liveSmoothing = 0.05

// smoothLive folds one reading into a direction's filtered rate. Each
// direction has its own accumulator, so the change of phase restarts
// the filter at zero instead of gliding down from the download figure.
func smoothLive(prev, sample float64) float64 {
	// A NaN or Inf sample would stick: every later reading folds into
	// it and comes back NaN/Inf, so the dial would stay blank for the
	// rest of the run. Negative or wildly large values are measurement
	// artefacts — drop them and keep the last good value instead.
	if math.IsNaN(sample) || math.IsInf(sample, 0) || sample < 0 || sample > 1e6 {
		return prev
	}
	return prev + liveSmoothing*(sample-prev)
}

// rttPhase holds one phase's latency samples twice over: as chart
// points, and as bare milliseconds.
//
// The duplication is deliberate. The chart wants (seconds, ms) pairs
// and the panel title wants a median, and pulling the Y values out of
// the points to compute one would allocate a slice on every frame. The
// sample count is in the tens, so carrying both costs nothing.
type rttPhase struct {
	Pts  []series.Point
	Vals []float64
}

// add records one sample: x is seconds since the run began, ms the
// round trip.
func (r *rttPhase) add(x, ms float64) {
	r.Pts = append(r.Pts, series.Point{X: x, Y: ms})
	r.Vals = append(r.Vals, ms)
}

// reset empties the phase, keeping the backing arrays for the next run.
func (r *rttPhase) reset() {
	r.Pts = r.Pts[:0]
	r.Vals = r.Vals[:0]
}

// len is the sample count.
func (r *rttPhase) len() int { return len(r.Vals) }
