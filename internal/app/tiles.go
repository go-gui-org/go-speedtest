package app

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"

	"github.com/go-gui-org/go-map/tile"
)

// Darkening curve, applied to every tile before it is drawn.
//
// The OSM basemap is rendered for paper: near-white land, pale blue
// water, white roads. In this window that made the map the brightest
// object on screen, and it pulled the eye off the readings. There is no
// keyless dark raster basemap left to switch to — CARTO and Stadia both
// want an API key now — so the tiles are re-toned here instead.
//
// Re-toning the tiles rather than dimming the widget is the whole point:
// a color filter over the map container would take the two pins and the
// route line down with the tiles, and there is no way to exempt them.
// Doing it at the source leaves the overlays at full strength, so they
// stay the brightest thing in the card.
//
// The curve is a gamma pushed above 1 and then a flat scale down. The
// gamma is what keeps the map readable: a plain multiply drags the
// pale-on-pale detail OSM relies on into a single flat grey, while
// deepening first spreads those tones apart before the whole thing is
// dimmed.
const (
	tileGamma = 1.25
	tileScale = 0.46
	// Channel trims that cool the result. Warm grey reads as a faded
	// paper map; a cool one reads as a dark map, and it matches the
	// blue the rest of the window is lit with.
	tileRedTrim  = 0.90
	tileBlueTint = 1.10
)

// darkTone is the curve above, precomputed per channel. A tile is 65536
// pixels and the viewport holds dozens of them, so this is three table
// lookups per pixel instead of three pow calls.
var darkTone = buildDarkTone()

func buildDarkTone() (t [3][256]uint8) {
	for i := range 256 {
		v := math.Pow(float64(i)/255, tileGamma) * tileScale
		t[0][i] = clamp8(v * tileRedTrim)
		t[1][i] = clamp8(v)
		t[2][i] = clamp8(v * tileBlueTint)
	}
	return t
}

// clamp8 converts a 0..1 intensity to a byte, holding the ends.
func clamp8(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 255
	default:
		return uint8(v*255 + 0.5)
	}
}

// darkTiles wraps a tile source and re-tones everything it returns.
//
// Only the HTTP fetcher is wrapped, because that is the path the map
// actually draws through: go-map hands the fetcher to go-gui, which
// caches the bytes that come back, so each tile is decoded and re-toned
// once rather than once per frame. Fetch and the metadata methods pass
// straight through to the embedded source, which keeps the OSM user
// agent and attribution intact.
type darkTiles struct {
	tile.Source
}

// toneMark distinguishes a re-toned tile's URL from the plain one.
//
// go-gui caches downloaded images on disk under a shared directory keyed
// by URL, so without this the re-toned tiles and the untouched ones any
// other go-gui program fetched from the same OSM URL land on the same
// cache entry — and whichever ran first wins. The tile server ignores
// the parameter and serves the same PNG.
const toneMark = "?tone=dark"

// URL marks the tile address so the re-toned bytes get their own entry
// in that cache. Empty stays empty: a source with no URL renders through
// Fetch instead, and there is nothing to key.
func (d darkTiles) URL(c tile.Coord) string {
	url := d.Source.URL(c)
	if url == "" {
		return ""
	}
	return url + toneMark
}

// HTTPFetcher returns the wrapped source's fetcher with a re-tone step
// on the end. If the inner source has no fetcher — no HTTP behind it —
// there is nothing to wrap and the map falls back to Fetch.
func (d darkTiles) HTTPFetcher() func(context.Context, string) (*http.Response, error) {
	inner, ok := d.Source.(tile.HTTPFetcher)
	if !ok {
		return nil
	}
	fetch := inner.HTTPFetcher()
	return func(ctx context.Context, url string) (*http.Response, error) {
		resp, err := fetch(ctx, url)
		if err != nil || resp.StatusCode != http.StatusOK {
			return resp, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		toned, err := darken(body)
		if err != nil {
			// A tile that will not decode is the tile server having a
			// bad day, not a reason to lose the map. Hand back what
			// arrived and let go-gui report it.
			toned = body
		}
		resp.Body = io.NopCloser(bytes.NewReader(toned))
		resp.ContentLength = int64(len(toned))
		return resp, nil
	}
}

// darken decodes a PNG tile, applies the tone curve, and re-encodes it.
func darken(src []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	// OSM serves paletted PNGs, so this is nearly always a conversion
	// rather than a copy. Going through NRGBA once is cheaper than
	// paying the palette indirection on every pixel below.
	b := img.Bounds()
	out, ok := img.(*image.NRGBA)
	if !ok {
		out = image.NewNRGBA(b)
		draw.Draw(out, b, img, b.Min, draw.Src)
	}
	px := out.Pix
	for i := 0; i < len(px); i += 4 {
		px[i] = darkTone[0][px[i]]
		px[i+1] = darkTone[1][px[i+1]]
		px[i+2] = darkTone[2][px[i+2]]
	}
	// Sized from the source: the re-encode lands within a few KiB of
	// the original, so this is one allocation instead of a growth chain.
	buf := bytes.NewBuffer(make([]byte, 0, len(src)+1024))
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
