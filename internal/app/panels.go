package app

import (
	"math"
	"strconv"
	"strings"
	"sync"

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

// livePanel stacks the two directions as separate charts rather than
// drawing both on one pair of axes. The pair splits the row evenly,
// so their height follows the row's, which the map beside them sets.
//
// One chart forced both directions onto one Y axis, so a 500 Mbps
// download flattened a 20 Mbps upload into the axis line. Two charts
// each scale to their own direction, which is the only way the upload
// curve has any shape at all on an asymmetric link. They share the
// vertical space the single chart had.
func livePanel(s *State) gui.View {
	down := s.Down.Snapshot()
	up := s.Up.Snapshot()

	return gui.Column(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			directionPanel("Download", "chart:live:down", down, colorDown),
			directionPanel("Upload", "chart:live:up", up, colorUp),
		},
	})
}

// directionPanel is one direction's sliding area chart.
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
func directionPanel(
	title, id string, data series.XY, accent gui.Color,
) gui.View {
	// An empty direction still draws its chart, not a placeholder.
	// A placeholder is Fit-height while a chart claims more, so the two
	// panels split the row unevenly and then jump to half each the
	// moment the second direction starts. Drawing an empty grid keeps
	// the two boxes the same size from the first frame to the last.
	yAxis, window := liveYAxis(), liveWindow(latestX(data))
	if len(data.Points) == 0 {
		yAxis, window = emptyYAxis(), minLiveSeconds
	}

	return panel(title+"  ·  Mbps", accent, chart.Area(chart.AreaCfg{
		BaseCfg: chart.BaseCfg{
			ID:      id,
			Sizing:  gui.FillFill,
			Version: seriesVersion(data),
			// One series, named in the panel title: a legend would
			// spend a corner of the plot repeating it.
			LegendPosition: &noLegend,
			// The hover tooltip names the units, not the axes: "X"
			// and "Y" say where a number sits, "sec" and "Mbps" say
			// what it is.
			TooltipXLabel: "sec",
			TooltipYLabel: "Mbps",
		},
		// No zoom, no pan, and no transition tween. The tween morphs the
		// old data into the new one on every version bump; at twenty
		// updates a second it never finishes, and the curve wobbles.
		InteractionCfg: chart.InteractionCfg{},
		Series:         []series.XY{data},
		LineWidth:      2,
		Opacity:        0.28,
		XAxis:          liveXAxis(),
		YAxis:          yAxis,
		AutoScroll:     true,
		// Each direction sizes its window to its own run, not to the
		// pair's. A shared width would give the finished download a
		// window as long as the whole test and leave a third of its
		// plot blank while the upload ran.
		WindowSize: window,
	}))
}

// seriesVersion is the redraw key for one direction's chart: the point
// count, which only changes when that direction gets a new reading.
//
// The app's own Version counter would do, but it ticks on every event
// in the run, so the finished download chart would redraw itself
// through the whole upload phase for nothing.
func seriesVersion(data series.XY) uint64 {
	return uint64(len(data.Points))
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
//
// AutoRange is what keeps the top label on screen. Linear.Ticks rounds
// the tick range outwards to nice numbers, so the highest tick sits
// above the data maximum; without AutoRange the domain stays at the
// data bounds and that tick is drawn above the plot, where the panel
// clips its label to a sliver. AutoRange widens the domain to the tick
// range instead, and rounding the top up never moves the zero floor:
// the floor is already a multiple of any spacing.
func liveYAxis() axis.Axis {
	// The format also reaches the hover tooltip, which otherwise
	// prints the raw float.
	return axis.NewLinear(axis.LinearCfg{
		AutoRange: true,
		// Exactly four, not the default eight. Each of these plots
		// gets half a row, and eight labels on that height read as a
		// solid grey column.
		//
		// TickCount rather than TickTarget: the range grows through the
		// whole run, and a target lets the label count flip between
		// three and four as it crosses a rounding boundary, which reads
		// as the axis twitching. The cost is headroom — the top tick is
		// raised to whatever four round steps need, so a 500 Mbps run
		// gets a 0-600 axis.
		TickCount:  4,
		TickFormat: mbpsTick,
	})
}

// emptyYAxis is the scale a direction shows before it has run, because
// an axis with no data has no range of its own and would draw a single
// "0" floating in an empty panel.
//
// The top is 150, not a rounder 100, so that four ticks fit inside the
// range: with a fixed range the domain does not follow the ticks, and
// four round steps over 0-100 put the last tick at 150, above the plot,
// where the panel clips its label in half.
func emptyYAxis() axis.Axis {
	return axis.NewLinear(axis.LinearCfg{
		Min: 0, Max: 150, TickCount: 4, TickFormat: mbpsTick,
	})
}

// mbpsTick labels a throughput axis.
//
// format.Mbps varies its precision with the size of the value, which is
// right for a single reading and wrong for a column of ticks: an upload
// axis came out as 140, 120, 100, 80.0, 60.0, which reads as two
// different scales stacked on each other.
func mbpsTick(v float64) string {
	// Ticks are produced by repeated addition of a nice spacing, so
	// 100.0 can arrive as 99.9999999998. Exact Trunc would then label
	// it "100.0" while its neighbour is "80".
	if math.Abs(v-math.Round(v)) < 1e-9 {
		return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
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
		TickTarget: 6,
		TickFormat: func(v float64) string {
			if v < 0 {
				return ""
			}
			return itoa(int(v)) + "s"
		},
	})
}

// gaugeThemeCache holds the trimmed theme between frames. Rebuilt only
// when the app theme changes, because theme.Default allocates on every
// call and the view tree is rebuilt many times a second.
var (
	gaugeThemeCache   *charttheme.Theme
	gaugeThemeCacheMu sync.Mutex
)

// Room the graduations and their labels need outside the arc.
// go-charts draws the marks outward from the arc and the numbers
// beyond them (chart/gauge.go drawTicks), so the dial gives up that
// much of the plot box. The two axes differ because the arc and its
// labels do not reach equally far in every direction: at 270° the
// widest labels sit almost dead level with the centre, while the
// topmost ones sit at 63° and so stand less far above it.
const (
	gaugeSidePad = 36
	gaugeEdgePad = 22
)

// gaugeTheme is the chart theme with the axis padding replaced by a
// label allowance. Every other chart needs the default 40/60px band
// for its tick labels; the gauge draws no axis, so most of that band
// is empty ground the dial could be using.
func gaugeTheme() *charttheme.Theme {
	bg := gui.CurrentTheme().ColorBackground
	gaugeThemeCacheMu.Lock()
	defer gaugeThemeCacheMu.Unlock()
	if gaugeThemeCache == nil || gaugeThemeCache.Background != bg {
		t := *charttheme.Default()
		// Top and bottom match so the dial centres itself in the
		// panel: go-charts puts the centre at the middle of the
		// padded box, so an uneven pair would push it off-centre and
		// leave the slack on one side.
		t.PaddingTop = gaugeEdgePad
		t.PaddingBottom = gaugeEdgePad
		t.PaddingRight = gaugeSidePad
		t.PaddingLeft = gaugeSidePad
		gaugeThemeCache = &t
	}
	return gaugeThemeCache
}

// gaugePanel is the speed dial. It tracks the live reading during a
// run and the headline figure afterwards, so it always shows the
// number the big tiles show.
func gaugePanel(s *State) gui.View {
	value, accent := gaugeValue(s)
	top, div, unit, valueFormat := gaugeRange(s)
	value /= div

	cfg := cardChrome()
	// Fixed and roughly square: the dial's radius is half the smaller
	// of the panel's two sides (chart/gauge.go), so any width past the
	// panel's height is empty ground around the dial. The latency
	// column takes that width instead.
	cfg.Sizing = gui.FixedFill
	cfg.Width = gaugeWidth
	cfg.Padding = gui.NewPadding(8, 10, 8, 10)
	cfg.Spacing = gui.SomeF(4)
	cfg.Content = []gui.View{
		panelTitle(gaugeTitle(s, unit), accent),
		chart.Gauge(chart.GaugeCfg{
			BaseCfg: chart.BaseCfg{
				ID:     "chart:gauge",
				Sizing: gui.FillFill,
				// A dial has no axes, so the default chart
				// padding (40/60px, reserved for tick labels)
				// is dead space that shrinks the radius.
				Theme: gaugeTheme(),
				// The zone legend does not fit beside a dial this
				// narrow, and it says nothing the colored arc does
				// not already say.
				LegendPosition: &noLegend,
				Version:        s.Version,
			},
			Value:       value,
			Min:         0,
			Max:         top,
			ShowValue:   true,
			ShowPointer: true,
			// The needle carries the direction, blue for download
			// and green for upload, so a glance at the dial says
			// which way the bytes are going without reading the
			// title. The zone colors under it keep saying how fast
			// the reading is.
			PointerColor: accent,
			// Six labelled graduations, a fifth of the range
			// apart, with a minor mark every quarter of that.
			// Enough to read the needle without counting, few
			// enough that the numbers do not run into each other on
			// a dial this size.
			TickCount:      5,
			MinorTicks:     3,
			ShowTickLabels: true,
			ValueFormat:    valueFormat,
			// The default holds back 15% of the radius for the
			// graduation labels. The theme padding above already
			// reserves that room, so leaving the default in place
			// would pay the same allowance twice and shrink the
			// dial for nothing.
			RadiusRatio: 1,
			// Three zones, not a continuous ramp: the point is to
			// tell "usable" from "fast" at a glance, and a gradient
			// makes every reading look mid-range. The boundaries
			// sit on graduations (200 and 600) so a zone edge never
			// falls between two labelled marks.
			Zones: []chart.GaugeZone{
				{Label: "Slow", Threshold: top * 0.2, Color: colorAlert},
				{Label: "Fair", Threshold: top * 0.6, Color: colorLatency},
				{Label: "Fast", Threshold: top, Color: accent},
			},
			// Blend the three zones into one ramp. Hard zone edges
			// drew two bright seams across the arc, which read as
			// marks on the dial rather than as a scale; the ramp
			// still goes red to amber to accent, so the same three
			// readings are still tellable apart.
			GradientZones: true,
		}),
	}
	return gui.Column(cfg)
}

// gaugeValue picks what the needle points at.
func gaugeValue(s *State) (float64, gui.Color) {
	switch {
	case s.Phase == probe.PhaseUpload:
		return s.LiveUp, colorUp
	case s.Phase.Active():
		return s.LiveDown, colorDown
	case s.Result != nil:
		return s.Result.DownMbps, colorDown
	default:
		return 0, colorDown
	}
}

// gaugeTitle names the direction on show and the unit the dial is
// counting in, which the range decides.
func gaugeTitle(s *State, unit string) string {
	if s.Phase == probe.PhaseUpload {
		return "Upload  ·  " + unit
	}
	return "Download  ·  " + unit
}

// gaugeMax is the top of the dial's base range, in Mbps, and the
// reading that switches it to the gigabit range.
//
// Two fixed ranges, not a scale that tracks the data. A continuously
// growing scale meant the same needle angle stood for a different speed
// from one run to the next, and the graduations relabelled themselves
// mid-test. Two ranges keep both properties that matter: within a range
// the angles are stable, and a 2.5 or 10 gigabit link still lands on
// the dial instead of pinning at the top.
const gaugeMax float64 = 1000

// gaugeMaxGbps is the top of the gigabit range, in Gbps. Ten covers
// every consumer fibre tier sold today.
const gaugeMaxGbps float64 = 10

// gaugeRange is the dial's scale: its maximum, the divisor that puts a
// reading in Mbps onto it, and the unit its labels are in.
//
// The gigabit range counts in Gbps rather than Mbps because "10000" and
// "8000" as graduation labels do not fit around a dial this size, and a
// four-digit needle reading is harder to take in at a glance than one
// decimal.
func gaugeRange(s *State) (top, div float64, unit, format string) {
	if s.HighRange {
		return gaugeMaxGbps, 1000, "Gbps", "%.1f"
	}
	return gaugeMax, 1, "Mbps", "%.0f"
}

// boxPanel shows the latency distribution as a box and whisker.
//
// go-charts computes the Tukey quartiles and the 1.5×IQR fences itself
// from the raw samples, which is exactly the input a latency test
// already has. The outlier dots are the interesting part: they are the
// stalls a mean would hide.
func boxPanel(s *State) gui.View {
	if len(s.RTTms) < 2 {
		return panel("Latency spread  ·  ms", colorLatency,
			placeholder("Collecting samples"))
	}

	return panel("Latency spread  ·  ms", colorLatency, chart.BoxPlot(chart.BoxPlotCfg{
		BaseCfg: chart.BaseCfg{
			ID:     "chart:box",
			Sizing: gui.FillFill,
			// The X-axis category label already names the box; a
			// legend would print it a second time.
			LegendPosition: &noLegend,
			Version:        s.Version,
		},
		YAxis: boxYAxis(s.RTTms),
		// A single box stretched across a wide panel reads as a bar
		// chart. Pin the body width and let the whiskers have the room.
		BoxWidth: 120,
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
		return panel("Latency distribution", colorLatency,
			placeholder("Collecting samples"))
	}

	return panel("Latency distribution", colorLatency, chart.Histogram(chart.HistogramCfg{
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

// boxYAxis sets the latency box plot's Y range and thins its labels.
//
// The range has to be set here: go-charts only calls SetRange on an
// axis it created itself (chart/boxplot.go), so a supplied axis that
// is never given a domain collapses the box to a flat line.
//
// The thinning is because go-charts asks for eight ticks whatever the
// panel is worth, and this panel is a third of a window tall: at a
// 20 ms spread that is a tick every 2 ms and the labels print on top of
// each other. All eight gridlines are still drawn — they are what makes
// the box readable — but only every labelStep'th value gets text.
func boxYAxis(vals []float64) axis.Axis {
	lo, hi := bounds(vals)
	step := labelStep(hi - lo)

	a := axis.NewLinear(axis.LinearCfg{
		AutoRange: true,
		TickFormat: func(v float64) string {
			// Rounded before the test: a tick value reached by
			// repeated addition of a nice spacing is not exact.
			if math.Abs(math.Remainder(v, step)) > step/100 {
				return ""
			}
			return format.MillisF(v)
		},
	})
	// The same 5% breathing room go-charts gives an axis of its own,
	// so a whisker that reaches the extreme still has a gap above it.
	pad := (hi - lo) * 0.05
	a.SetRange(lo-pad, hi+pad)
	return a
}

// bounds is the smallest and largest finite value in vals. An empty,
// all-NaN or all-Inf input gives a unit range, which keeps the axis
// drawable.
func bounds(vals []float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		lo, hi = min(lo, v), max(hi, v)
	}
	if math.IsInf(lo, 1) || lo == hi {
		return 0, 1
	}
	return lo, hi
}

// labelStep picks the spacing between labelled ticks: the smallest
// familiar round number that keeps the label count near five.
func labelStep(span float64) float64 {
	if !(span > 0) {
		return 1
	}
	for _, step := range []float64{1, 2, 5, 10, 20, 50, 100, 200, 500} {
		if span/step <= 5 {
			return step
		}
	}
	return 1000
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

// connPanel is the who-and-where of this run: which address family the
// transfers used, which datacenter answered, whose network the client
// is on, and the address the edge saw. None of it is a measurement, but
// it is what turns a number into a result you can act on — a slow run
// through a satellite ISP and a slow run to a datacenter two continents
// away are different problems.
func connPanel(s *State) gui.View {
	if s.Trace == nil {
		return panel("Connection", colorDown,
			placeholder("Waiting for the edge…"))
	}
	t := s.Trace

	rows := []gui.View{
		connRow(gui.IconSitemap, "Connected via", ipFamily(t.IP), colorDown),
		connRow(gui.IconBuilding, "Server location", coloPlace(t), colorUp),
		connRow(gui.IconShare, "Your network", networkName(t), colorLatency),
		connRow(gui.IconCreditCard, "Your IP address", orDash(t.IP), colorDown),
	}
	if t.HTTPProtocol != "" {
		// Worth a line of its own: HTTP/1.1 caps a single connection's
		// throughput in a way HTTP/2 and HTTP/3 do not, so the protocol
		// is sometimes the whole explanation for the number above.
		rows = append(rows,
			connRow(gui.IconLock, "Protocol", t.HTTPProtocol, colorUp))
	}

	cfg := cardChrome()
	cfg.Sizing = gui.FixedFill
	cfg.Width = connPanelWidth
	cfg.Padding = gui.NewPadding(8, 10, 8, 10)
	cfg.Spacing = gui.SomeF(10)
	cfg.Content = append(
		[]gui.View{panelTitle("Connection", colorDown)}, rows...)
	return gui.Column(cfg)
}

// connRow is one labelled fact: a colored icon, the label above, and
// the value below it in the panel's strongest text.
//
// Two lines rather than the one line Cloudflare's own page uses,
// because a network name runs to forty characters and this column is
// narrower than a browser window. The value wraps instead of being cut
// off: the interesting half of "MAQUOKETA VALLEY ELECTRIC COOPERATIVE"
// is not the first twenty characters.
func connRow(icon, label, value string, accent gui.Color) gui.View {
	theme := gui.CurrentTheme()

	iconStyle := theme.Icon3
	iconStyle.Color = accent

	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFit,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(8),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: icon, TextStyle: iconStyle}),
			gui.Column(gui.ContainerCfg{
				Sizing:     gui.FillFit,
				Padding:    gui.NoPadding,
				Spacing:    gui.SomeF(1),
				SizeBorder: gui.NoBorder,
				Content: []gui.View{
					gui.Text(gui.TextCfg{
						Text: label, TextStyle: theme.TextStyleLabel,
					}),
					gui.Text(gui.TextCfg{
						Text:      value,
						TextStyle: theme.B4,
						Sizing:    gui.FillFit,
						Mode:      gui.TextModeWrap,
					}),
				},
			}),
		},
	})
}

// ipFamily names the address family from the address itself. A colon
// can only appear in an IPv6 literal, so no parsing is needed.
func ipFamily(ip string) string {
	switch {
	case ip == "":
		return "—"
	case strings.Contains(ip, ":"):
		return "IPv6"
	default:
		return "IPv4"
	}
}

// coloPlace names the answering datacenter: its city when we know it,
// its IATA code otherwise, and both when they differ in usefulness.
func coloPlace(t *probe.Trace) string {
	switch {
	case t.ColoCity != "" && t.Colo != "":
		return t.ColoCity + " (" + t.Colo + ")"
	case t.ColoCity != "":
		return t.ColoCity
	default:
		return orDash(t.Colo)
	}
}

// networkName is the client's ISP with its autonomous system number,
// which is the part a support ticket actually needs.
func networkName(t *probe.Trace) string {
	if t.ASOrg == "" {
		if t.ASN == 0 {
			return "—"
		}
		return "AS" + itoa(t.ASN)
	}
	if t.ASN == 0 {
		return t.ASOrg
	}
	return t.ASOrg + "  (AS" + itoa(t.ASN) + ")"
}

// orDash keeps an empty field from rendering as a blank line, which
// reads as a layout bug rather than as missing data.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
