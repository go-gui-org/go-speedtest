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
