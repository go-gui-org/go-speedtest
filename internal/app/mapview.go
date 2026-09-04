package app

import (
	"math"

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
//
// The tiles are re-toned on the way in; see darkTiles.
func TileSource() tile.Source {
	return darkTiles{Source: tile.OSMWithUserAgent(probe.DefaultUserAgent)}
}

// mapPanel is the map card: where the traffic went.
func mapPanel(s *State) gui.View {
	cfg := cardChrome()
	cfg.Sizing = gui.FixedFill
	cfg.Width = mapPanelWidth
	cfg.Padding = gui.NewPadding(8, 10, 8, 10)
	cfg.Spacing = gui.SomeF(4)
	cfg.Content = []gui.View{
		panelTitle(mapTitle(s), colorUp),
		mapview.Map(mapview.Cfg{
			ID:        mapID,
			Sizing:    gui.FillFill,
			Focusable: true,
			Source:    s.Tiles,
			// A world view until the trace lands, at which point
			// applyTrace frames the two pins.
			InitialCenter: projection.LatLng{Lat: 20, Lng: 0},
			InitialZoom:   1,
			// What shows through before a tile arrives. Left at the
			// default it was a pale flash on every pan; matching the
			// re-toned tiles means a missing one reads as empty rather
			// than as a hole.
			Background: mix(gui.CurrentTheme().ColorPanel, colorDown, 0.05),
			A11YLabel:  "Map of the route to the serving datacenter",
		}),
	}
	return gui.Column(cfg)
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
