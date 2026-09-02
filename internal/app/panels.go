package app

import (
	"github.com/go-gui-org/go-charts/axis"
	"github.com/go-gui-org/go-charts/chart"
	"github.com/go-gui-org/go-charts/series"
	charttheme "github.com/go-gui-org/go-charts/theme"
	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-speedtest/internal/format"
	"github.com/go-gui-org/go-speedtest/internal/probe"
	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// liveSeconds is the widest the live chart's time window gets. Wide
// enough to hold a whole transfer direction, narrow enough that the
// curve still has visible shape.
const liveSeconds = 22

// minLiveSeconds is the narrowest it gets, at the start of a run.
const minLiveSeconds = 3

// noLegend hides a chart's legend. Taken by address because
// BaseCfg.LegendPosition is a pointer: nil means "use the theme".
var noLegend = charttheme.LegendNone

// livePanel is the sliding area chart: download and upload on one
// continuous timeline.
//
// AutoScroll is what stops the chart jumping. Without it the X axis
// re-derives its domain from the data on every frame, so each arriving
// point compresses the whole curve horizontally and the drawing shifts
// under itself. AutoScroll instead holds a fixed-width window and
// tweens its right edge, so a new point moves the line and nothing
// else.
//
// Supplying XAxis does not pin anything: go-charts calls SetRange on a
// caller-supplied axis with the data bounds (autoLinearAxis in
// chart/xyaxes.go), and only the AutoScroll path marks a domain as
// overridden. The axis is passed here for its tick format alone.
func livePanel(s *State) gui.View {
	down := s.Down.Snapshot()
	up := s.Up.Snapshot()

	if len(down.Points) == 0 && len(up.Points) == 0 {
		return panel("Throughput", placeholder("Press Start test"))
	}

	// An empty series still draws a legend entry, so only pass the ones
	// that have data.
	data := make([]series.XY, 0, 2)
	if len(down.Points) > 0 {
		data = append(data, down)
	}
	if len(up.Points) > 0 {
		data = append(data, up)
	}

	return panel("Throughput  ·  Mbps", chart.Area(chart.AreaCfg{
		BaseCfg: chart.BaseCfg{
			ID:      "chart:live",
			Sizing:  gui.FillFill,
			Version: s.Version,
		},
		// No zoom, no pan, and no transition tween. The tween morphs the
		// old data into the new one on every version bump; at twenty
		// updates a second it never finishes, and the curve wobbles.
		InteractionCfg: chart.InteractionCfg{},
		Series:         data,
		LineWidth:      2,
		Opacity:        0.28,
		XAxis:          liveXAxis(),
		YAxis:          liveYAxis(),
		AutoScroll:     true,
		WindowSize:     liveWindow(latestX(down, up)),
	}))
}

// liveWindow is the width of the visible time window.
//
// It grows with the run and then stops. AutoScroll always puts the
// newest point at the right edge, so a window as wide as the run keeps
// the left edge at zero and the curve filling the panel. Once the run
// is longer than liveSeconds the width stops growing and the window
// starts to slide, dropping the oldest readings off the left.
func liveWindow(latest float64) float64 {
	if latest != latest || latest < 0 {
		return minLiveSeconds
	}
	return min(max(latest, minLiveSeconds), liveSeconds)
}

// liveYAxis exists to suppress padding, not to set a range.
//
// go-charts pads an auto-created Y axis by 5% below the data, which
// puts the floor of a throughput chart at a negative rate. Supplying an
// axis skips that padding (autoLinearAxis in chart/xyaxes.go applies
// padFrac only when it creates the axis itself), so the floor lands on
// the zero anchor the event pump appends at the start of each
// direction.
func liveYAxis() axis.Axis {
	// The format also reaches the hover tooltip, which otherwise
	// prints the raw float.
	return axis.NewLinear(axis.LinearCfg{TickFormat: format.Mbps})
}

// liveXAxis labels the time window in seconds.
//
// In the first seconds of a run the window is wider than the run is
// old, so its left edge sits before the run started. Those ticks are
// left blank: the run has no data there and labelling a negative second
// would be a lie. The range itself comes from AutoScroll, not from this
// axis.
func liveXAxis() axis.Axis {
	return axis.NewLinear(axis.LinearCfg{
		TickFormat: func(v float64) string {
			if v < 0 {
				return ""
			}
			return itoa(int(v)) + "s"
		},
	})
}

// gaugePanel is the speed dial. It tracks the live reading during a
// run and the headline figure afterwards, so it always shows the
// number the big tiles show.
func gaugePanel(s *State) gui.View {
	value, accent := gaugeValue(s)
	scale := gaugeScale(s)

	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FixedFill,
		Width:   gaugeWidth,
		Color:   gui.CurrentTheme().ColorPanel,
		Radius:  gui.SomeF(gui.CurrentTheme().RadiusSmall),
		Padding: gui.NewPadding(8, 10, 8, 10),
		Spacing: gui.SomeF(4),
		Content: []gui.View{
			gui.Text(gui.TextCfg{
				Text:      gaugeTitle(s),
				TextStyle: gui.CurrentTheme().TextStyleLabel,
			}),
			chart.Gauge(chart.GaugeCfg{
				BaseCfg: chart.BaseCfg{
					ID:     "chart:gauge",
					Sizing: gui.FillFill,
					// The zone legend does not fit beside a dial this
					// narrow, and it says nothing the colored arc does
					// not already say.
					LegendPosition: &noLegend,
					Version:        s.Version,
				},
				Value:       value,
				Min:         0,
				Max:         scale,
				ShowValue:   true,
				ShowPointer: true,
				ValueFormat: "%.0f",
				// Three zones, not a continuous ramp: the point is to
				// tell "usable" from "fast" at a glance, and a gradient
				// makes every reading look mid-range.
				Zones: []chart.GaugeZone{
					{Label: "Slow", Threshold: scale * 0.25, Color: colorAlert},
					{Label: "Fair", Threshold: scale * 0.6, Color: colorLatency},
					{Label: "Fast", Threshold: scale, Color: accent},
				},
			}),
		},
	})
}

// gaugeValue picks what the needle points at.
func gaugeValue(s *State) (float64, gui.Color) {
	switch {
	case s.Phase == probe.PhaseUpload:
		return s.Live, colorUp
	case s.Phase.Active():
		return s.Live, colorDown
	case s.Result != nil:
		return s.Result.DownMbps, colorDown
	default:
		return 0, colorDown
	}
}

func gaugeTitle(s *State) string {
	if s.Phase == probe.PhaseUpload {
		return "Upload  ·  Mbps"
	}
	return "Download  ·  Mbps"
}

// gaugeScale keeps the needle on the dial for any link speed.
//
// A fixed maximum would either clip gigabit fibre or leave a phone
// tethering at the very bottom of the arc. The scale climbs through
// familiar round numbers and never shrinks within a direction, so the
// needle does not jump backwards when a reading dips.
//
// The scale follows the direction being measured. A shared scale would
// hold the download peak through the upload phase, and a normal
// asymmetric link would then show its upload sitting in the red.
func gaugeScale(s *State) float64 {
	peak := max(s.PeakDown, s.Live)
	if s.Phase == probe.PhaseUpload {
		peak = max(s.PeakUp, s.Live)
	}
	if peak != peak || peak < 0 {
		return 25
	}
	for _, step := range []float64{25, 50, 100, 250, 500, 1000, 2500} {
		if peak <= step*0.9 {
			return step
		}
	}
	return 5000
}

// boxPanel shows the latency distribution as a box and whisker.
//
// go-charts computes the Tukey quartiles and the 1.5×IQR fences itself
// from the raw samples, which is exactly the input a latency test
// already has. The outlier dots are the interesting part: they are the
// stalls a mean would hide.
func boxPanel(s *State) gui.View {
	if len(s.RTTms) < 2 {
		return panel("Latency spread  ·  ms", placeholder("Collecting samples"))
	}

	return panel("Latency spread  ·  ms", chart.BoxPlot(chart.BoxPlotCfg{
		BaseCfg: chart.BaseCfg{
			ID:     "chart:box",
			Sizing: gui.FillFill,
			// The X-axis category label already names the box; a
			// legend would print it a second time.
			LegendPosition: &noLegend,
			Version:        s.Version,
		},
		Data: []chart.BoxData{{
			Label:  boxLabel(s),
			Values: s.RTTms,
			Color:  colorLatency,
		}},
	}))
}

// boxLabel puts the two numbers a box plot does not draw — the sample
// count and p95 — where the axis label already is.
func boxLabel(s *State) string {
	return "n=" + itoa(len(s.RTTms)) +
		"  p95 " + format.MillisF(stats.Percentile(s.RTTms, 0.95))
}

// histogramPanel shows the same samples as a shape rather than a
// summary: a tight single peak is a healthy link, a second peak to the
// right is a retransmit or a busy queue.
func histogramPanel(s *State) gui.View {
	if len(s.RTTms) < 3 {
		return panel("Latency distribution", placeholder("Collecting samples"))
	}

	return panel("Latency distribution", chart.Histogram(chart.HistogramCfg{
		BaseCfg: chart.BaseCfg{
			ID:      "chart:hist",
			Sizing:  gui.FillFill,
			Version: s.Version,
		},
		// Re-binning every frame is fine here: the sample count is in
		// the tens, and the run is bounded by the network, not by this.
		Data:       s.RTTms,
		Color:      colorLatency,
		Radius:     2,
		TickFormat: format.MillisF,
	}))
}

// latestX is the largest X value present in either series, which is
// how far into the run the chart has data for.
func latestX(sets ...series.XY) float64 {
	var latest float64
	for _, set := range sets {
		if n := len(set.Points); n > 0 {
			v := set.Points[n-1].X
			// A corrupt point must not swing the viewport off screen.
			if v == v && v >= 0 && v < 1e9 {
				latest = max(latest, v)
			}
		}
	}
	return latest
}

// itoa avoids pulling strconv into a file that otherwise only formats
// measurements.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		n = -n
		if n < 0 {
			// Most-negative int: deterministic fallback.
			return "0"
		}
		var buf [21]byte
		buf[0] = '-'
		i := len(buf)
		for n > 0 {
			i--
			buf[i] = byte('0' + n%10)
			n /= 10
		}
		return "-" + string(buf[i:])
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
