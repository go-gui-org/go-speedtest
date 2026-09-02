package app

import "github.com/go-gui-org/go-gui/gui"

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

// styleColor returns style with its color replaced. Used where a
// reading needs its series color rather than the theme's text color.
func styleColor(style gui.TextStyle, c gui.Color) gui.TextStyle {
	style.Color = c
	return style
}
