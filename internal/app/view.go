package app

import (
	"fmt"
	"time"

	glyph "github.com/go-gui-org/go-glyph"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-speedtest/internal/format"
	"github.com/go-gui-org/go-speedtest/internal/probe"
	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// Panel sizes. The second row takes whatever height is left, so only
// the hero band needs a number.
const (
	heroRowHeight float32 = 340
	mapPanelWidth float32 = 380
	gaugeWidth    float32 = 340
	// The connection facts are text, so the column is as wide as a
	// readable line of it and no wider; the charts want the rest.
	connPanelWidth float32 = 250
)

// Root is the window's view generator, registered once in OnInit and
// re-run by the event pump through UpdateWindow.
func Root(w *gui.Window) gui.View {
	s := state(w)
	theme := gui.CurrentTheme()

	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FillFill,
		Color:   theme.ColorBackground,
		Padding: gui.NoPadding,
		Content: []gui.View{
			headerView(s),
			heroRow(s),
			secondRow(s),
		},
	})
}

// headerView is the title bar: what this is, where it is measuring to,
// the mascot while a run is live, and the one control.
func headerView(s *State) gui.View {
	theme := gui.CurrentTheme()

	left := []gui.View{
		gui.Text(gui.TextCfg{Text: "go-speedtest", TextStyle: theme.B2}),
		gui.Text(gui.TextCfg{Text: statusLine(s), TextStyle: theme.N5}),
	}
	// The mascot is a self-driving animated SVG: it registers its own
	// render-only ticker while it is in the tree, so it needs nothing
	// from the app but a place to stand.
	if s.Phase.Active() {
		left = append(left, gui.SvgSpinner(gui.SvgSpinnerCfg{
			ID:      "mascot",
			Kind:    gui.SvgSpinnerBarsScaleMiddle,
			Width:   26,
			Height:  26,
			Color:   colorDown,
			A11YCfg: gui.A11YCfg{A11YLabel: "Test running"},
		}))
	}

	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFit,
		Color:      theme.ColorPanel,
		Padding:    gui.NewPadding(10, 16, 10, 16),
		Spacing:    gui.SomeF(12),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gui.Row(gui.ContainerCfg{
				Sizing:     gui.FillFit,
				Padding:    gui.NoPadding,
				Spacing:    gui.SomeF(12),
				VAlign:     gui.VAlignMiddle,
				SizeBorder: gui.NoBorder,
				Content:    left,
			}),
			runButton(s),
		},
	})
}

// runButton starts or stops a run. One control, because one control is
// all a speed test needs.
func runButton(s *State) gui.View {
	label := "Start test"
	if s.Running() {
		label = "Stop"
	}
	return gui.Button(gui.ButtonCfg{
		ID:      "run",
		Label:   label,
		Variant: gui.ButtonPrimary,
		Width:   110,
		OnClick: func(ctx gui.EventCtx) {
			ctx.Consume()
			if state(ctx.Window).Running() {
				Stop(ctx.Window)
			} else {
				Start(ctx.Window)
			}
			// The click callback does not hold the frame lock, so this
			// redraw is safe here; without it the button label would
			// not change until the first event arrived.
			ctx.Window.UpdateWindow()
		},
	})
}

// statusLine describes the run in one phrase, including the two states
// that are easy to miss: a failure, and a run that used no network.
func statusLine(s *State) string {
	switch {
	case s.Err != nil:
		return "failed: " + s.Err.Error()
	case s.Phase == probe.PhaseIdle && s.Result == nil:
		return "Ready"
	case s.Phase.Active():
		return s.Phase.Label() + "…" + traceSuffix(s)
	case s.Result != nil && s.Result.Simulated:
		return "Complete (simulated, no network used)" + traceSuffix(s)
	case s.Warn != nil:
		return s.Phase.Label() + ", with a problem: " + s.Warn.Error()
	default:
		return s.Phase.Label() + traceSuffix(s)
	}
}

// traceSuffix names the answering datacenter once it is known.
func traceSuffix(s *State) string {
	if s.Trace == nil || s.Trace.Colo == "" {
		return ""
	}
	if s.Trace.ColoKnown {
		return fmt.Sprintf("  ·  %s (%s)", s.Trace.ColoCity, s.Trace.Colo)
	}
	return "  ·  " + s.Trace.Colo
}

// heroRow is what the eye lands on first: the dial on the left, the
// headline numbers beside it, and the latency distribution on the
// right. The chart and the map wait for the second row.
func heroRow(s *State) gui.View {
	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFixed,
		Height:     heroRowHeight,
		Padding:    gui.NewPadding(10, 12, 8, 12),
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gaugePanel(s),
			statsPanel(s),
			latencyColumn(s),
			connPanel(s),
		},
	})
}

// statsPanel is the headline numbers as plain text, not cards. Cards
// gave every reading a box the width of the window; the numbers are
// what matters, so they get the ink and nothing else does.
func statsPanel(s *State) gui.View {
	theme := gui.CurrentTheme()

	// While a run is going the tiles track the latest reading of each
	// direction. Not the peak: an upload peak is a socket-buffer
	// artifact, and showing it would state a speed the link cannot
	// reach.
	down, up := s.LiveDown, s.LiveUp
	if s.Result != nil {
		down, up = s.Result.DownMbps, s.Result.UpMbps
	}

	return gui.Column(gui.ContainerCfg{
		// Fit, not Fill: the column is only as wide as its widest
		// reading, which leaves the slack in the row for the dial.
		Sizing:     gui.FitFill,
		Padding:    gui.NewPadding(4, 8, 4, 8),
		Spacing:    gui.SomeF(10),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			statText("Download", format.Mbps(down), "Mbps", colorDown),
			statText("Upload", format.Mbps(up), "Mbps", colorUp),
			statText("Latency", format.MillisF(stats.Median(s.RTTms)), "ms p50", colorLatency),
			statText("Jitter", format.MillisF(stats.Jitter(s.RTTms)), "ms", colorLatency),
			statText("Elapsed", elapsedText(s), "", theme.B2.Color),
		},
	})
}

// Stat readout geometry. The value box is a fixed width so the column
// does not breathe as digits come and go; the bar, the label and the
// number all start on its left edge.
const (
	statValueSize  float32 = 30
	statValueWidth float32 = 116
	statBarWidth   float32 = 4
	statRowHeight  float32 = 48
)

// statText is one headline reading: an accent bar, a small label, and
// the number in the series color with its unit beside it.
func statText(label, value, unit string, accent gui.Color) gui.View {
	theme := gui.CurrentTheme()

	// The mono face, not the proportional one: every digit is the
	// same width, so a live reading does not shuffle sideways as 199
	// becomes 200. Size is set outright because the theme's ladder
	// tops out well below what a hero number wants.
	style := theme.M1
	style.Size = statValueSize
	style.Color = accent
	// The glyphs fade from a lit tint at the top to the flat series
	// color at the bottom, which is what keeps a wall of numbers from
	// reading as a spreadsheet.
	style.Gradient = &glyph.GradientConfig{
		Direction: glyph.GradientVertical,
		Stops: []glyph.GradientStop{
			{Color: glyphColor(lighten(accent, 0.5)), Position: 0},
			{Color: glyphColor(accent), Position: 1},
		},
	}

	// The unit rides on the label line, not beside the number. Beside
	// it, the unit sits wherever that reading's digits end, so a short
	// value opens a gap the eye reads as a missing column. Up here
	// every line starts on the same left edge, bar included.
	head := []gui.View{
		gui.Text(gui.TextCfg{Text: label, TextStyle: theme.TextStyleLabel}),
	}
	if unit != "" {
		// The label's own size, one step dimmer. The secondary style
		// is a size larger, which made the unit shout over the reading
		// it belongs to.
		unitStyle := theme.TextStyleLabel
		unitStyle.Color = unitStyle.Color.WithOpacity(0.7)
		head = append(head, gui.Text(gui.TextCfg{
			Text: unit, TextStyle: unitStyle,
		}))
	}

	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FitFit,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(10),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			// The bar carries the series color at full strength, which
			// lets the eye group a reading with its curve on the chart
			// below without reading either label.
			gui.Rectangle(gui.RectangleCfg{
				Sizing: gui.FixedFixed,
				Width:  statBarWidth,
				Height: statRowHeight,
				Radius: statBarWidth / 2,
				Color:  accent,
				Gradient: &gui.GradientDef{
					Type:      gui.GradientLinear,
					Direction: gui.GradientToBottom,
					Stops: []gui.GradientStop{
						{Color: lighten(accent, 0.5), Pos: 0},
						{Color: accent, Pos: 1},
					},
				},
			}),
			gui.Column(gui.ContainerCfg{
				Sizing:     gui.FitFit,
				Padding:    gui.NoPadding,
				Spacing:    gui.SomeF(0),
				SizeBorder: gui.NoBorder,
				Content: []gui.View{
					gui.Row(gui.ContainerCfg{
						Sizing:     gui.FitFit,
						Padding:    gui.NoPadding,
						Spacing:    gui.SomeF(6),
						VAlign:     gui.VAlignBottom,
						SizeBorder: gui.NoBorder,
						Content:    head,
					}),
					// Fixed width, not Fit: the readings change several
					// times a second, and a Fit box would re-widen the
					// whole column every time a digit is gained or lost,
					// shoving the charts beside it sideways.
					gui.Row(gui.ContainerCfg{
						Sizing:     gui.FixedFit,
						Width:      statValueWidth,
						Padding:    gui.NoPadding,
						SizeBorder: gui.NoBorder,
						Content: []gui.View{
							gui.Text(gui.TextCfg{Text: value, TextStyle: style}),
						},
					}),
				},
			}),
		},
	})
}

// elapsedText shows the running clock while a test is live and the
// final duration afterwards.
func elapsedText(s *State) string {
	switch {
	case s.Result != nil:
		return s.Result.Duration.Round(100 * time.Millisecond).String()
	case s.Phase.Active():
		return time.Since(s.Started).Round(time.Second).String()
	default:
		return "—"
	}
}

// secondRow is the two views that need room to breathe: the live
// throughput chart and the map of where the traffic went. It takes all
// the height the hero row leaves.
func secondRow(s *State) gui.View {
	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NewPadding(0, 12, 12, 12),
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			livePanel(s),
			mapPanel(s),
		},
	})
}

// latencyColumn stacks the two views of the same samples, because a
// box plot and a histogram answer different questions.
func latencyColumn(s *State) gui.View {
	return gui.Column(gui.ContainerCfg{
		// Fill: this column takes whatever width the square gauge
		// panel and the fit-width stats leave.
		Sizing:     gui.FillFill,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			boxPanel(s),
			histogramPanel(s),
		},
	})
}

// panel wraps a chart in the standard card: a title above, the chart
// filling the rest.
func panel(title string, body gui.View) gui.View {
	theme := gui.CurrentTheme()
	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FillFill,
		Color:   theme.ColorPanel,
		Radius:  gui.SomeF(theme.RadiusSmall),
		Padding: gui.NewPadding(8, 10, 8, 10),
		Spacing: gui.SomeF(4),
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: title, TextStyle: theme.TextStyleLabel}),
			body,
		},
	})
}

// placeholder fills a panel that has no data yet, so the layout does
// not jump when the first samples arrive.
func placeholder(text string) gui.View {
	theme := gui.CurrentTheme()
	return gui.Column(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NoPadding,
		HAlign:     gui.HAlignCenter,
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: text, TextStyle: theme.TextStylePlaceholder}),
		},
	})
}
