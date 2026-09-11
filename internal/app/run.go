package app

import (
	"math"
	"sync"
	"time"

	"github.com/go-gui-org/go-charts/series"
	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

// Start begins a run, replacing any run already in progress.
//
// Called on the window thread — from OnInit or a button callback — so
// it touches State directly.
func Start(w *gui.Window) {
	s := state(w)

	// A custom URL is checked before anything is torn down. Refusing
	// here leaves the last run's numbers on screen, which is what the
	// user wants to keep looking at while they fix the address.
	p := s.Provider()
	if p.Custom {
		if err := probe.CheckURL(s.CustomURL); err != nil {
			s.ProviderErr = err
			return
		}
	}
	s.ProviderErr = nil
	// The provider fills in the protocol, the origin and the server
	// together; everything else on the config is this app's own.
	cfg := p.Apply(probe.Config{Timeout: s.Timeout}, s.CustomURL, s.ServerIdx)

	// Retire the previous run before clearing the charts, not after:
	// between the two the old pump is still appending, and anything it
	// writes in that window survives the reset.
	ctx, gen := s.beginRun()
	s.reset()
	s.Started = time.Now()
	s.Phase = probe.PhaseTrace

	eng := probe.New(cfg)
	go pump(w, s, eng.Run(ctx), s.Started, gen)
}

// Stop cancels the run in progress and leaves whatever was measured on
// screen.
func Stop(w *gui.Window) {
	s := state(w)
	// endRun, not a bare cancel: the engine's channel still holds
	// events, and without retiring the generation the chart would keep
	// growing for a moment after the button was pressed.
	if !s.endRun() {
		return
	}
	if s.Phase.Active() {
		s.Phase = probe.PhaseIdle
	}
}

// pump drains the engine's events and publishes them to the window.
//
// It runs on its own goroutine and never takes the window lock.
// w.Lock from another goroutine panics if it lands while a frame is
// being built, which at this event rate it eventually will;
// QueueCommand is the supported cross-thread path and runs the
// mutation on the window thread instead.
//
// The RealTimeSeries appends are the exception: those carry their own
// mutex, so they happen here rather than being deferred to a frame.
func pump(w *gui.Window, s *State, events <-chan probe.Event, started time.Time, gen uint64) {
	pub := &publisher{w: w, s: s, gen: gen}

	for ev := range events {
		// A run that has been replaced or stopped still has events in
		// the channel behind it. They belong to a chart that is gone,
		// so this pump goes quiet the moment its generation is retired
		// rather than writing them into the run that took its place.
		if !s.isCurrentRun(gen) {
			continue
		}
		switch ev.Kind {
		case probe.EventPhase:
			phase := ev.Phase
			// Anchor the series at zero when a transfer direction
			// starts. Nothing had moved at that instant, so the point
			// is true, and it makes the area meet the baseline instead
			// of floating above it.
			switch phase {
			case probe.PhaseDownload:
				s.Down.Append(series.Point{X: time.Since(started).Seconds()})
			case probe.PhaseUpload:
				s.Up.Append(series.Point{X: time.Since(started).Seconds()})
			}
			pub.post(func(s *State) { s.Phase = phase })

		case probe.EventTrace:
			tr := ev.Trace
			pub.post(func(s *State) { s.Trace = tr })
			// Map overlays are window state, so they go through the
			// same queue rather than being touched from here.
			w.QueueCommand(func(w *gui.Window) {
				// Checked again on the window thread: the queue is
				// drained a frame later, by which time this run may
				// have been replaced, and the pins would land on the
				// new run's map.
				if s.isCurrentRun(gen) {
					applyTrace(w, tr)
				}
			})

		case probe.EventRTT:
			ms := float64(ev.RTT) / float64(time.Millisecond)
			// Which phase a sample came from is the whole point of
			// collecting it: idle and loaded latency are different
			// readings and go in different boxes.
			phase := ev.Phase
			// Seconds since the run began, the same clock the rate
			// events use, so the latency chart lines up with the
			// throughput charts above it.
			x := time.Since(started).Seconds()
			pub.post(func(s *State) {
				switch phase {
				case probe.PhaseDownload:
					s.RTTDown.add(x, ms)
				case probe.PhaseUpload:
					s.RTTUp.add(x, ms)
				default:
					s.RTTIdle.add(x, ms)
				}
				s.Version++
			})

		case probe.EventRate:
			// X is seconds since the run began, not since the phase
			// began, so download and upload share one timeline.
			x := time.Since(started).Seconds()
			mbps := ev.Mbps
			phase := ev.Phase

			// A corrupt reading must not pollute the chart's domain
			// (which tracks data bounds) or the filtered live value.
			if math.IsNaN(mbps) || math.IsInf(mbps, 0) || mbps < 0 || mbps > 1e6 {
				break
			}
			if x < 0 || x > 1e9 {
				break
			}

			if phase == probe.PhaseUpload {
				s.Up.Append(series.Point{X: x, Y: mbps})
			} else {
				s.Down.Append(series.Point{X: x, Y: mbps})
			}

			pub.post(func(s *State) {
				if phase == probe.PhaseUpload {
					s.LiveUp = smoothLive(s.LiveUp, mbps)
				} else {
					s.LiveDown = smoothLive(s.LiveDown, mbps)
				}
				// Latched off the filtered value, so a single spike
				// cannot move the whole dial to the gigabit range.
				if s.LiveDown > gaugeMax || s.LiveUp > gaugeMax {
					s.HighRange = true
				}
				s.Version++
			})

		case probe.EventWarning:
			warn := ev.Err
			pub.post(func(s *State) { s.Warn = warn })

		case probe.EventError:
			err := ev.Err
			pub.post(func(s *State) { s.Err = err })
			s.finishRun(gen)

		case probe.EventDone:
			res := ev.Res
			pub.post(func(s *State) {
				s.Result = res
				s.Version++
			})
			s.finishRun(gen)
		}
	}
}

// publisher batches state changes onto the window thread.
//
// While a command is waiting to run, further changes join it instead of
// queueing a command of their own. That coalesces naturally to the
// frame rate: a fast engine and a slow frame produce one command
// carrying many changes, not a backlog of commands.
type publisher struct {
	w *gui.Window
	s *State
	// gen is the run this publisher belongs to. A batch is dropped
	// rather than applied once that run has been retired.
	gen uint64

	mu      sync.Mutex
	pending []func(*State)
	queued  bool
}

// post schedules mutate to run on the window thread.
func (p *publisher) post(mutate func(*State)) {
	p.mu.Lock()
	p.pending = append(p.pending, mutate)
	first := !p.queued
	p.queued = true
	p.mu.Unlock()

	if !first {
		return
	}
	p.w.QueueCommand(func(w *gui.Window) {
		p.mu.Lock()
		batch := p.pending
		p.pending = nil
		p.queued = false
		p.mu.Unlock()

		// The batch was built before this command reached the window
		// thread, so its run can have been replaced in between. The
		// whole batch belongs to one run, so one check covers it.
		if !p.s.isCurrentRun(p.gen) {
			return
		}
		for _, fn := range batch {
			fn(p.s)
		}
		// Safe inside a queued command: InvalidateLayout only marks the
		// window for refresh and wakes the loop, it takes no lock.
		w.InvalidateLayout()
	})
}
