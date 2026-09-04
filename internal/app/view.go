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
		Sizing: gui.FillFill,
		Color:  theme.ColorBackground,
		// A wash down the window, so the cards at the top sit against
		// a slightly different ground than the ones at the bottom. On
		// a flat fill every card had the same contrast against the
		// same shade, which is what made the window read as a table.
		Gradient: &gui.GradientDef{
			Type:      gui.GradientLinear,
			Direction: gui.GradientToBottom,
			Stops: []gui.GradientStop{
				{Color: lighten(theme.ColorBackground, 0.04), Pos: 0},
				{Color: theme.ColorBackground, Pos: 1},
			},
		},
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

	bar := gui.Row(gui.ContainerCfg{
		Sizing: gui.FillFit,
		Color:  theme.ColorPanel,
		// Lit from the left in the download color. The ramp used to
		// land on the bare panel color, which reads as the light
		// running out into black by mid-bar. It now stops at the tint
		// the old ramp had about a third of the way across and holds
		// it to the right edge, so the whole bar stays lit and only
		// the strength of the light changes.
		Gradient: &gui.GradientDef{
			Type:      gui.GradientLinear,
			Direction: gui.GradientToRight,
			Stops: []gui.GradientStop{
				{Color: mix(theme.ColorPanel, colorDown, headerTintNear), Pos: 0},
				{Color: mix(theme.ColorPanel, colorDown, headerTintFar), Pos: 1},
			},
		},
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
			providerPicker(s),
			runButton(s),
		},
	})

	return gui.Column(gui.ContainerCfg{
		Sizing:     gui.FillFit,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(0),
		SizeBorder: gui.NoBorder,
		Content:    []gui.View{bar, headerRule()},
	})
}

// How much of the download color the title bar catches at each end.
// The far value is not zero on purpose: it is roughly where the old
// ramp sat a third of the way across, which keeps a trace of the
// light on the right side instead of fading out to bare panel.
const (
	headerTintNear = 0.16
	headerTintFar  = 0.06
)

// headerRuleHeight is a hairline, not a band: the rule is there to
// close the header off, not to be looked at.
const headerRuleHeight float32 = 2

// headerRule is the download-to-upload sweep under the header. It is
// the one place the two direction colors are shown together, which
// says up front what the whole window is measuring.
func headerRule() gui.View {
	return gui.Rectangle(gui.RectangleCfg{
		Sizing: gui.FillFixed,
		Height: headerRuleHeight,
		Color:  colorDown,
		Gradient: &gui.GradientDef{
			Type:      gui.GradientLinear,
			Direction: gui.GradientToRight,
			Stops: []gui.GradientStop{
				{Color: colorDown, Pos: 0},
				{Color: colorUp, Pos: 0.6},
				{Color: colorLatency, Pos: 1},
			},
		},
	})
}

// Picker geometry. The provider combobox is wide enough for the
// longest provider name plus its arrow; the URL field is wide enough
// for a hostname without being wide enough to crowd the button. The
// server combobox is wider than both because its entries carry a city,
// a country and a sponsor.
const (
	pickerWidth    float32 = 165
	customURLWidth float32 = 260
	serverWidth    float32 = 250
)

// providerPicker chooses where a run is pointed.
//
// It is in the title bar rather than in a settings panel because it
// changes what the next run measures, which puts it next to the button
// that starts one. Locked while a run is in flight: switching provider
// mid-run would leave the numbers on screen belonging to a host the
// header no longer names.
//
// Two entries reveal a second control beside the picker: the custom one
// a URL field, and a provider that publishes servers a second combobox
// to choose between them. Both are only built when their provider is
// selected, so the header stays one control wide for the providers that
// need no address.
func providerPicker(s *State) gui.View {
	running := s.Running()
	names := make([]string, len(s.Providers))
	for i, p := range s.Providers {
		names[i] = p.Name
	}

	content := []gui.View{
		gui.Combobox(gui.ComboboxCfg{
			ID:      "provider",
			Options: names,
			// The list is taller than MaxDropdownHeight, and an
			// unscrollable dropdown paints its overflow outside the
			// panel instead of clipping it.
			Scrollable: true,
			Value:      s.Provider().Name,
			MinWidth:   pickerWidth,
			MaxWidth:   pickerWidth,
			Disabled:   running,
			A11YCfg:    gui.A11YCfg{A11YLabel: "Speed test provider"},
			OnSelect: func(name string, ctx gui.EventCtx) {
				st := state(ctx.Window)
				for i, p := range st.Providers {
					if p.Name == name {
						st.ProviderIdx = i
						break
					}
				}
				// A new provider makes the old complaint stale, and
				// the entries that need no URL can never have one.
				st.ProviderErr = nil
				// The index belonged to the old provider's list. Server
				// lists differ in length, so carrying it over would
				// silently select a different host than the one the
				// picker last showed.
				st.ServerIdx = 0
				ctx.Window.UpdateWindow()
			},
		}),
	}

	// Which server, for a provider that has more than one. A list of
	// one would be a control with no choice in it, so it is skipped.
	if servers := s.Provider().Servers; len(servers) > 1 {
		names := make([]string, len(servers))
		for i, srv := range servers {
			names[i] = srv.Name
		}
		content = append(content, gui.Combobox(gui.ComboboxCfg{
			ID:      "server",
			Options: names,
			// Server lists run to dozens of entries; without this the
			// rows below the cap draw over the window (see above).
			Scrollable: true,
			Value:      s.Server().Name,
			MinWidth:   serverWidth,
			MaxWidth:   serverWidth,
			Disabled:   running,
			A11YCfg:    gui.A11YCfg{A11YLabel: "Speed test server"},
			OnSelect: func(name string, ctx gui.EventCtx) {
				st := state(ctx.Window)
				for i, srv := range st.Provider().Servers {
					if srv.Name == name {
						st.ServerIdx = i
						break
					}
				}
				ctx.Window.UpdateWindow()
			},
		}))
	}

	if s.Provider().Custom {
		content = append(content, gui.Input(gui.InputCfg{
			ID:          "provider-url",
			Text:        s.CustomURL,
			Placeholder: "https://host.example",
			Width:       customURLWidth,
			Disabled:    running,
			A11YCfg:     gui.A11YCfg{A11YLabel: "Custom provider base URL"},
			OnTextChanged: func(text string, ctx gui.EventCtx) {
				st := state(ctx.Window)
				st.CustomURL = text
				// Cleared on edit, not re-checked: complaining about a
				// half-typed URL after every keystroke is noise. The
				// check runs once, when Start is pressed.
				st.ProviderErr = nil
			},
		}))
	}

	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FitFit,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(8),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content:    content,
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
	case s.ProviderErr != nil:
		return "provider: " + s.ProviderErr.Error()
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

// traceSuffix names the answering host once it is known.
//
// The code in brackets is Cloudflare's datacenter identifier. A
// LibreSpeed server has no such code, so it is dropped rather than
// filled in with something that only looks like one.
func traceSuffix(s *State) string {
	switch {
	case s.Trace == nil:
		return ""
	case s.Trace.ColoCity != "" && s.Trace.Colo != "":
		return fmt.Sprintf("  ·  %s (%s)", s.Trace.ColoCity, s.Trace.Colo)
	case s.Trace.ColoCity != "":
		return "  ·  " + s.Trace.ColoCity
	case s.Trace.Colo != "":
		return "  ·  " + s.Trace.Colo
	default:
		return ""
	}
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
			statText("Latency", format.MillisF(stats.Median(s.RTTIdle.Vals)), "ms p50", colorLatency),
			statText("Jitter", format.MillisF(stats.Jitter(s.RTTIdle.Vals)), "ms", colorLatency),
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

// latencyColumn is the latency plot, filling the height two shorter
// charts used to share. One tall panel beats two short ones here: the
// samples run from tens of milliseconds to the occasional stall in the
// seconds, and that range needs the height to stay readable.
func latencyColumn(s *State) gui.View {
	return latencyPanel(s)
}

// panel wraps a chart in the standard card: a title above, the chart
// filling the rest. The accent colors the title chip, which is the
// only place a card states which reading it belongs to.
func panel(title string, accent gui.Color, body gui.View) gui.View {
	cfg := cardChrome()
	cfg.Sizing = gui.FillFill
	cfg.Padding = gui.NewPadding(8, 10, 8, 10)
	cfg.Spacing = gui.SomeF(4)
	cfg.Content = []gui.View{panelTitle(title, accent), body}
	return gui.Column(cfg)
}

// placeholder fills a panel that has no data yet, so the layout does
// not jump when the first samples arrive.
// busyPlaceholder is what a panel shows while its chart is still being
// measured: a spinner and a line of text, centred in the card.
//
// The spinner is self-driving — it registers its own render ticker
// while it is in the tree — so it costs the app nothing but a place to
// stand.
func busyPlaceholder(text string, kind gui.SvgSpinnerKind, accent gui.Color) gui.View {
	theme := gui.CurrentTheme()
	return gui.Column(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(10),
		HAlign:     gui.HAlignCenter,
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gui.SvgSpinner(gui.SvgSpinnerCfg{
				ID:   "spinner:" + text,
				Kind: kind,
				// Large: this fills a panel, not a line of text, and
				// the pulse rings fade as they expand, so a small one
				// is barely there.
				Width:   72,
				Height:  72,
				Color:   accent,
				A11YCfg: gui.A11YCfg{A11YLabel: text},
			}),
			gui.Text(gui.TextCfg{
				Text: text, TextStyle: theme.TextStylePlaceholder,
			}),
		},
	})
}

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
