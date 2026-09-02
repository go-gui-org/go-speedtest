package app

import (
	"fmt"
	"time"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-speedtest/internal/format"
	"github.com/go-gui-org/go-speedtest/internal/probe"
	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// Panel heights. The middle row takes whatever is left, so only the
// fixed bands need a number.
const (
	bottomRowHeight float32 = 210
	mapPanelWidth   float32 = 380
	gaugeWidth      float32 = 190
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
			statsRow(s),
			middleRow(s),
			bottomRow(s),
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

// statsRow is the headline numbers. They read as a row of tiles so the
// eye can compare them without reading axes.
func statsRow(s *State) gui.View {
	theme := gui.CurrentTheme()

	// While a run is going the tiles track the latest reading of each
	// direction. Not the peak: an upload peak is a socket-buffer
	// artifact, and showing it would state a speed the link cannot
	// reach.
	down, up := s.LiveDown, s.LiveUp
	if s.Result != nil {
		down, up = s.Result.DownMbps, s.Result.UpMbps
	}

	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFit,
		Color:      theme.ColorPanel,
		Padding:    gui.NewPadding(10, 16, 12, 16),
		Spacing:    gui.SomeF(10),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			statTile("Download", format.Mbps(down), "Mbps", colorDown),
			statTile("Upload", format.Mbps(up), "Mbps", colorUp),
			statTile("Latency", format.MillisF(stats.Median(s.RTTms)), "ms p50", colorLatency),
			statTile("Jitter", format.MillisF(stats.Jitter(s.RTTms)), "ms", colorLatency),
			statTile("Elapsed", elapsedText(s), "", theme.B2.Color),
		},
	})
}

// statTile is one headline reading: a small label, a large value, and a
// unit the value does not have to repeat.
func statTile(label, value, unit string, accent gui.Color) gui.View {
	theme := gui.CurrentTheme()
	content := []gui.View{
		gui.Text(gui.TextCfg{Text: value, TextStyle: styleColor(theme.B2, accent)}),
	}
	if unit != "" {
		content = append(content, gui.Text(gui.TextCfg{
			Text: unit, TextStyle: theme.TextStyleSecondary,
		}))
	}

	return gui.Column(gui.ContainerCfg{
		Sizing:  gui.FillFit,
		Color:   theme.ColorInterior,
		Radius:  gui.SomeF(theme.RadiusSmall),
		Padding: gui.NewPadding(8, 12, 8, 12),
		Spacing: gui.SomeF(2),
		Content: []gui.View{
			gui.Text(gui.TextCfg{Text: label, TextStyle: theme.TextStyleLabel}),
			gui.Row(gui.ContainerCfg{
				Sizing:     gui.FillFit,
				Padding:    gui.NoPadding,
				Spacing:    gui.SomeF(5),
				VAlign:     gui.VAlignBottom,
				SizeBorder: gui.NoBorder,
				Content:    content,
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

// middleRow is the live throughput chart beside the map.
func middleRow(s *State) gui.View {
	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NewPadding(0, 12, 8, 12),
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			livePanel(s),
			mapPanel(s),
		},
	})
}

// bottomRow is the latency distribution: the same samples shown two
// ways, because a box plot and a histogram answer different questions.
func bottomRow(s *State) gui.View {
	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFixed,
		Height:     bottomRowHeight,
		Padding:    gui.NewPadding(0, 12, 12, 12),
		Spacing:    gui.SomeF(10),
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gaugePanel(s),
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
