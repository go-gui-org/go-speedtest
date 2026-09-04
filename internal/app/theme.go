package app

import (
	glyph "github.com/go-gui-org/go-glyph"
	"github.com/go-gui-org/go-gui/gui"
)

// Data-visualization colors.
//
// The theme deliberately supplies no chart palette: chart color is a
// data decision, not a chrome decision. These four are fixed and picked
// to stay legible on both the light and the dark panel.
var (
	colorDown    = gui.RGB(80, 170, 245)  // download: blue
	colorUp      = gui.RGB(120, 205, 130) // upload: green
	colorLatency = gui.RGB(235, 175, 75)  // latency and jitter: amber
	colorAlert   = gui.RGB(230, 95, 95)   // errors: red
)

// lighten raises a color's brightness and drops a little of its
// saturation, keeping the hue. A gradient between a color and its
// lightened self reads as one color lit unevenly, where a gradient
// between two palette colors reads as two different readings.
func lighten(c gui.Color, amount float32) gui.Color {
	h, sat, v := c.ToHSV()
	return gui.ColorFromHSV(h, sat*(1-amount/2), v+(1-v)*amount)
}

// glyphColor converts a color for the text renderer, which takes
// go-glyph's own color type rather than the toolkit's.
func glyphColor(c gui.Color) glyph.Color {
	return glyph.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// Card chrome. The panels were flat rectangles in one shade, which
// left the window reading as a spreadsheet: nothing sat in front of
// anything else. These give every card the same faint lift — a wash
// from top to bottom, a hairline edge, and a soft shadow under it —
// so the eye reads the panels as objects on a ground rather than as
// regions of one surface.
const (
	// How much lighter the top of a card is than its bottom. Small
	// on purpose: enough to suggest a light source, not enough to
	// read as a colored panel.
	cardLift = 0.05
	// The shadow is offset down and blurred wide, which is what
	// separates "raised card" from "outlined box".
	cardShadowOffsetY = 3
	cardShadowBlur    = 12
)

// cardChrome is the shared card surface: the gradient, the shadow and
// the border, ready to be spread into a container config.
//
// Returned as a config rather than a wrapper view because the panels
// differ in sizing, padding and content, and only the surface is
// common to all of them.
func cardChrome() gui.ContainerCfg {
	theme := gui.CurrentTheme()
	top := lighten(theme.ColorPanel, cardLift)
	return gui.ContainerCfg{
		Color:  theme.ColorPanel,
		Radius: gui.SomeF(theme.RadiusMedium),
		Gradient: &gui.GradientDef{
			Type:      gui.GradientLinear,
			Direction: gui.GradientToBottom,
			Stops: []gui.GradientStop{
				{Color: top, Pos: 0},
				{Color: theme.ColorPanel, Pos: 1},
			},
		},
		Shadow: &gui.BoxShadow{
			Color:      gui.RGBA(0, 0, 0, 90),
			OffsetY:    cardShadowOffsetY,
			BlurRadius: cardShadowBlur,
		},
	}
}

// Title chip geometry: a short bar, not a dot, because a bar echoes
// the accent bars beside the headline numbers.
const (
	chipWidth  float32 = 3
	chipHeight float32 = 12
)

// panelTitle is a card's heading: a colored chip, then the words. The
// chip carries the same color as the data in the card, so a glance
// pairs a panel with its reading in the hero row without reading
// either label.
func panelTitle(title string, accent gui.Color) gui.View {
	theme := gui.CurrentTheme()
	return gui.Row(gui.ContainerCfg{
		Sizing:     gui.FillFit,
		Padding:    gui.NoPadding,
		Spacing:    gui.SomeF(7),
		VAlign:     gui.VAlignMiddle,
		SizeBorder: gui.NoBorder,
		Content: []gui.View{
			gui.Rectangle(gui.RectangleCfg{
				Sizing: gui.FixedFixed,
				Width:  chipWidth,
				Height: chipHeight,
				Radius: chipWidth / 2,
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
			gui.Text(gui.TextCfg{
				Text: title, TextStyle: theme.TextStyleLabel,
			}),
		},
	})
}

// mix blends b into a by amount (0 keeps a, 1 gives b). Used to tint a
// chrome color towards a data color by a few percent, which reads as
// the same surface catching some of the accent rather than as a second
// surface.
func mix(a, b gui.Color, amount float32) gui.Color {
	blend := func(x, y uint8) uint8 {
		return uint8(float32(x) + (float32(y)-float32(x))*amount)
	}
	return gui.RGBA(
		blend(a.R, b.R), blend(a.G, b.G), blend(a.B, b.B),
		max(a.A, 1),
	)
}
