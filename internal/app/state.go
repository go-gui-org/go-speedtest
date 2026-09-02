// Package app is the dashboard: window state, the view tree, and the
// bridge between the measurement engine and the screen.
//
// The split from internal/probe is deliberate. Nothing here measures
// anything, and nothing there knows a window exists.
package app

import (
	"context"
	"sync"
	"time"

	"github.com/go-gui-org/go-charts/chart"
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

	// RTTms holds latency samples in milliseconds, in collection
	// order. Collection order matters: jitter is defined over
	// consecutive samples.
	RTTms []float64

	Phase  probe.Phase
	Trace  *probe.Trace
	Result *probe.Result
	// Err is a fatal error: the run stopped. Warn is a phase that
	// ended early while the run carried on.
	Err     error
	Warn    error
	Started time.Time

	// Live is the most recent reading of whichever direction is being
	// measured. It drives the gauge.
	Live float64

	// LiveDown and LiveUp are the most recent reading of each
	// direction. The stat tiles show these while a run is going, so
	// the download figure stays on screen through the upload phase.
	LiveDown float64
	LiveUp   float64

	// PeakDown and PeakUp are the largest readings seen. They size the
	// gauge and nothing else: an upload peak is inflated by socket
	// buffering, so it must never be shown as a measurement.
	PeakDown float64
	PeakUp   float64

	// Demo and Timeout are the flags a new run is started with.
	Demo    bool
	Timeout time.Duration

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
	// mapFitted guards the one-time map framing, so a redraw does not
	// yank a viewport the user has since panned.
	mapFitted bool
}

// New builds the initial state.
func New(demo bool, timeout time.Duration, tiles tile.Source) *State {
	return &State{
		Down:    chart.NewRealTimeSeries(chart.RealTimeSeriesCfg{Name: "Download", Color: colorDown, MaxLen: seriesCapacity}),
		Up:      chart.NewRealTimeSeries(chart.RealTimeSeriesCfg{Name: "Upload", Color: colorUp, MaxLen: seriesCapacity}),
		Demo:    demo,
		Timeout: timeout,
		Tiles:   tiles,
		Phase:   probe.PhaseIdle,
	}
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

// setCancel installs a new canceller and cancels whatever it replaced,
// so a second Start never leaves the first run orphaned.
func (s *State) setCancel(c context.CancelFunc) {
	s.cancelMu.Lock()
	prev := s.cancel
	s.cancel = c
	s.cancelMu.Unlock()
	if prev != nil {
		prev()
	}
}

// clearCancel cancels the run in progress, if any, and reports whether
// there was one.
func (s *State) clearCancel() bool {
	s.cancelMu.Lock()
	c := s.cancel
	s.cancel = nil
	s.cancelMu.Unlock()
	if c == nil {
		return false
	}
	c()
	return true
}

// reset clears everything a previous run left behind. Called on the
// window thread before a new run starts.
func (s *State) reset() {
	s.Down.Clear()
	s.Up.Clear()
	s.RTTms = s.RTTms[:0]
	s.Trace = nil
	s.Result = nil
	s.Err = nil
	s.Warn = nil
	s.Live = 0
	s.LiveDown = 0
	s.LiveUp = 0
	s.PeakDown = 0
	s.PeakUp = 0
	s.mapFitted = false
	s.Version++
}
