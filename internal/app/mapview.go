package app

import (
	"math"

	glyph "github.com/go-gui-org/go-glyph"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-map/mapview"
	"github.com/go-gui-org/go-map/projection"
	"github.com/go-gui-org/go-map/tile"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

// mapID is the map's widget ID. Overlay and viewport calls address the
// map by it, so it is a constant rather than a literal repeated at
// every call site.
const mapID = "map"

// Overlay IDs. Stable, because AddOverlay replaces by ID: re-adding the
// same marker updates it instead of stacking a second one.
const (
	overlayClient = "client"
	overlayColo   = "colo"
	overlayLink   = "link"
)

// singlePinZoom frames one pin with its region around it: close enough
// that the city is recognisable, far enough that the pin is placed in a
// country rather than on a street. The coordinates behind it are a city
// centre or a country centroid, so anything closer would claim a
// precision the data does not have.
const singlePinZoom = 5

// arcSegments is how many points the great-circle link is sampled at.
// go-map draws straight segments between vertices, so the curve is
// this sampling; 48 is smooth at any zoom the app uses.
const arcSegments = 48

// TileSource is the map's tile provider, built once in main and shared
// with the window so the HTTP fetcher carries the same identifying
// user agent. OSM's tile policy requires that.
func TileSource() tile.Source {
	return tile.OSMWithUserAgent(probe.DefaultUserAgent)
}

// mapPanel is the map card: where the traffic went.
func mapPanel(s *State) gui.View {
	cfg := cardChrome()
	cfg.Sizing = gui.FixedFill
	cfg.Width = mapPanelWidth
	cfg.Padding = gui.NewPadding(8, 10, 8, 10)
	cfg.Spacing = gui.SomeF(4)
	cfg.Content = []gui.View{
		panelTitleID(mapTitle(s), colorUp, mapTitleID),
		mapHoverRing(s),
	}
	return gui.Column(cfg)
}

// Wheel-zoom cue. Whether the wheel zooms the map or scrolls the body
// is decided by one rule now: the pointer is over the map, so the map
// gets the event.
//
// Focus used to be a second rule, and a bad one. go-gui offers a
// scroll to the focused widget's handler before it looks under the
// cursor (gui.mouseScrollHandler), and go-map's handler takes it — so
// a click on the map captured the wheel for the whole window until
// something else took focus. The map is no longer focusable, which
// costs it its tab stop and its keyboard pan/zoom and buys back a
// wheel that always obeys the pointer. TestMapIsNotFocusable pins it.
//
// The cue says exactly that one rule: while the "Route" label is
// bold, the wheel zooms the map.
// The label goes bold and picks up the map's accent, and a hairline
// appears around the map rect — the label is the loud half, because a
// border alone was too easy to miss at a glance.
const (
	// mapTitleID addresses the card's label so the hover handler can
	// find it in the laid-out tree.
	mapTitleID = "map-title"
	// The ring is always this wide, lit or not. Widening it on hover
	// would re-run layout under the pointer and shift the map by a
	// pixel every time the cursor crossed the edge.
	mapRingWidth float32 = 1
	// How much of the map's accent the lit ring carries.
	mapRingTint = 0.45
)

// mapHoverRing wraps the map in the cue described above.
//
// The sensor is this container rather than the card, so it covers the
// rect the wheel actually acts on: the title strip above the map
// scrolls the body like any other chrome, and it stays outside.
func mapHoverRing(s *State) gui.View {
	theme := gui.CurrentTheme()
	return gui.Column(gui.ContainerCfg{
		Sizing:     gui.FillFill,
		Padding:    gui.NoPadding,
		SizeBorder: gui.SomeF(mapRingWidth),
		// At rest the ring is the card's own color, so the map looks
		// exactly as it did before the pointer arrived.
		ColorBorder: theme.ColorPanel,
		Radius:      gui.SomeF(theme.RadiusSmall),
		// OnHover runs inside the layout pass and paints into this
		// frame; nothing fires once the pointer leaves, and neither
		// shape is touched by a frame that lights nothing, so both
		// come back as the generator built them. No hover state to
		// keep, and none to clear.
		OnHover: func(c gui.EventCtx) {
			lightCue(c, theme)
		},
		Content: []gui.View{
			mapview.Map(mapview.Cfg{
				ID:     mapID,
				Sizing: gui.FillFill,
				// Deliberately not focusable — see the cue note above.
				// Focus would let the map keep the wheel after a click
				// with the pointer anywhere on screen.
				Focusable: false,
				Source:    s.Tiles,
				// A world view until the trace lands, at which point
				// applyTrace frames the two pins.
				InitialCenter: projection.LatLng{Lat: 20, Lng: 0},
				InitialZoom:   1,
				// What shows through before a tile arrives. Left at the
				// default it was a pale flash on every pan; a
				// panel-toned ground means a missing tile reads as
				// empty rather than as a hole.
				Background: mix(theme.ColorPanel, colorDown, 0.05),
				A11YLabel:  "Map of the route to the serving datacenter",
			}),
		},
	})
}

// lightCue lights both halves of the cue for this frame: the ring
// around the map and the card's label above it.
//
// c is the ring's own context either way, so the ring is c's shape and
// the label is a sibling one level up. The label is addressed by its
// effective ID, because the scrolling body is ID-bearing and scopes
// every ID under it ("body-scroll:map-title"). A miss is not worth
// handling — the ring still marks the map.
func lightCue(c gui.EventCtx, theme gui.Theme) {
	if c.Layout == nil || c.Layout.Shape == nil || c.Layout.Parent == nil {
		return
	}
	c.Layout.Shape.ColorBorder = mix(theme.ColorPanel, colorUp, mapRingTint)
	if title, ok := c.Layout.Parent.FindByID(c.EffID(mapTitleID)); ok {
		emphasize(title.Shape)
	}
}

// emphasize restyles a laid-out label bold and in the map's accent.
//
// It runs after the sizing pass, so the label keeps the box measured
// for the regular face and the wider glyphs simply draw into the free
// space to its right. Nothing else sits on that line, so there is
// nothing for them to collide with, and the row does not reflow under
// the pointer.
//
// The style is replaced, never edited in place: the pointer a label
// carries is the theme's own style, shared by every other label in the
// window.
func emphasize(sh *gui.Shape) {
	if sh == nil || sh.TC == nil || sh.TC.TextStyle == nil {
		return
	}
	st := *sh.TC.TextStyle
	st.Typeface = glyph.TypefaceBold
	st.Color = colorUp
	sh.TC.TextStyle = &st
}

// coloLabel names the far end: the city, with Cloudflare's datacenter
// code in brackets when there is one. A LibreSpeed server has no such
// code, so it gets the city alone.
func coloLabel(t *probe.Trace) string {
	if t.ColoCity != "" && t.Colo != "" {
		return t.ColoCity + " (" + t.Colo + ")"
	}
	if t.ColoCity != "" {
		return t.ColoCity
	}
	return t.Colo
}

// mapTitle names what the map is showing, which changes as the run
// learns where it is talking to.
func mapTitle(s *State) string {
	switch {
	case s.Trace == nil:
		return "Route"
	case s.Trace.ColoKnown:
		return "Route  ·  " + s.Trace.ColoCity
	default:
		return "Route  ·  " + s.Trace.Colo
	}
}

// applyTrace puts the two pins and the link on the map.
//
// It runs on the window thread through QueueCommand, never from the
// pump goroutine: AddOverlay and FitBounds mutate window state, and
// doing that from another thread is what the frame lock exists to
// prevent.
func applyTrace(w *gui.Window, tr *probe.Trace) {
	if tr == nil {
		return
	}
	s := state(w)

	var pts []projection.LatLng

	if tr.ClientKnown {
		client := projection.LatLng{Lat: tr.ClientLat, Lng: tr.ClientLng}
		pts = append(pts, client)
		mapview.AddOverlay(w, mapID, &mapview.Marker{
			MarkerID: overlayClient,
			Pos:      client,
			Label:    "Your location",
			Title:    "You",
			// Country centroid, not a street address: the app does no
			// geo-IP lookup and sends the address nowhere.
			Body:  "Approximate, from country code " + tr.Loc,
			Color: colorUp,
		})
	}

	if tr.ColoKnown {
		colo := projection.LatLng{Lat: tr.ColoLat, Lng: tr.ColoLng}
		pts = append(pts, colo)
		mapview.AddOverlay(w, mapID, &mapview.Marker{
			MarkerID: overlayColo,
			Pos:      colo,
			Label:    "Serving datacenter " + tr.ColoCity,
			Title:    coloLabel(tr),
			// Deliberately not "the Cloudflare edge": the same pin
			// now marks a LibreSpeed server too.
			Body:  "The server that answered this test",
			Color: colorDown,
		})
	}

	if len(pts) == 2 {
		mapview.AddOverlay(w, mapID, &mapview.Polyline{
			LineID:      overlayLink,
			Points:      greatCircle(pts[0], pts[1], arcSegments),
			StrokeColor: colorDown,
			StrokeWidth: 2,
			Label:       "Route from you to the serving datacenter",
		})
	}

	// Frame the pins once. Doing it on every redraw would fight a user
	// who has panned or zoomed since.
	if !s.mapFitted && len(pts) > 0 {
		cw, ch, ok := mapview.CanvasSize(w, mapID)
		if ok {
			if len(pts) == 1 {
				// One pin has no extent, and FitBounds on a zero-sized
				// box zooms all the way in — past the tile server's
				// maximum, so every tile request comes back 400 and the
				// card draws empty. A fixed regional zoom instead.
				//
				// This is the normal case for a LibreSpeed run: most of
				// the public servers have no IP database behind them,
				// so there is no client pin to pair with the server.
				mapview.SetView(w, mapID, pts[0], singlePinZoom)
			} else {
				mapview.FitBounds(w, mapID, boundsOf(pts), 48, cw, ch)
			}
			s.mapFitted = true
		}
	}
	w.UpdateWindow()
}

// boundsOf returns the smallest box covering every point.
func boundsOf(pts []projection.LatLng) projection.Bounds {
	if len(pts) == 0 {
		return projection.Bounds{}
	}
	b := projection.Bounds{NE: pts[0], SW: pts[0]}
	for _, p := range pts[1:] {
		b.NE.Lat = math.Max(b.NE.Lat, p.Lat)
		b.NE.Lng = math.Max(b.NE.Lng, p.Lng)
		b.SW.Lat = math.Min(b.SW.Lat, p.Lat)
		b.SW.Lng = math.Min(b.SW.Lng, p.Lng)
	}
	return b
}

// greatCircle samples the shortest path over the sphere between two
// points.
//
// A straight line in Web Mercator is not the route a packet takes, and
// at continental distances the difference is obvious enough to look
// wrong. Spherical linear interpolation on the unit vectors gives the
// real arc; go-map draws the samples as a polyline.
func greatCircle(a, b projection.LatLng, segments int) []projection.LatLng {
	if segments <= 0 {
		segments = 1
	}
	if segments > 256 {
		segments = 256
	}
	// Clamp degenerate coordinates before trig.
	if a.Lat != a.Lat || b.Lat != b.Lat || a.Lng != a.Lng || b.Lng != b.Lng {
		return []projection.LatLng{a, b}
	}
	ax, ay, az := toUnitVector(a)
	bx, by, bz := toUnitVector(b)

	// Angle between the two directions.
	dot := math.Min(1, math.Max(-1, ax*bx+ay*by+az*bz))
	omega := math.Acos(dot)
	if omega < 1e-9 {
		// Same point, or close enough that the arc is a dot.
		return []projection.LatLng{a, b}
	}
	sinOmega := math.Sin(omega)

	out := make([]projection.LatLng, 0, segments+1)
	for i := 0; i <= segments; i++ {
		t := float64(i) / float64(segments)
		ca := math.Sin((1-t)*omega) / sinOmega
		cb := math.Sin(t*omega) / sinOmega
		x, y, z := ax*ca+bx*cb, ay*ca+by*cb, az*ca+bz*cb
		out = append(out, projection.LatLng{
			Lat: math.Atan2(z, math.Hypot(x, y)) * 180 / math.Pi,
			Lng: math.Atan2(y, x) * 180 / math.Pi,
		})
	}
	return out
}

// toUnitVector converts a lat/lng to a point on the unit sphere.
func toUnitVector(p projection.LatLng) (x, y, z float64) {
	lat := p.Lat * math.Pi / 180
	lng := p.Lng * math.Pi / 180
	return math.Cos(lat) * math.Cos(lng),
		math.Cos(lat) * math.Sin(lng),
		math.Sin(lat)
}
