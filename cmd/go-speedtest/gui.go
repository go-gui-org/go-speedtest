package main

import (
	"context"
	"time"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-gui/gui/backend"
	"github.com/go-gui-org/go-map/tile"
	"github.com/go-gui-org/go-speedtest/internal/app"
)

// runGUI opens the dashboard and blocks until the window closes.
func runGUI(ctx context.Context, demo bool, timeout time.Duration, autostart bool) error {
	gui.SetTheme(gui.ThemeDark)

	src := app.TileSource()
	st := app.New(demo, timeout, src)

	cfg := gui.WindowCfg{
		State:  st,
		Title:  "go-speedtest",
		Width:  1180,
		Height: 780,
		OnInit: func(w *gui.Window) {
			// Register the view once. Every later redraw goes through
			// UpdateWindow, which keeps the map's viewport and the
			// charts' animation state.
			w.UpdateView(app.Root)
			if autostart {
				app.Start(w)
			}
		},
	}
	// The map fetches its tiles through the window's image fetcher, so
	// the tile source's user agent reaches the tile server. OSM's usage
	// policy requires an identifying agent.
	if f, ok := src.(tile.HTTPFetcher); ok {
		cfg.ImageFetcher = f.HTTPFetcher()
	}

	w := gui.NewWindow(cfg)

	// Ctrl-C in the launching terminal stops a run in progress; the
	// window itself stays open.
	go func() {
		<-ctx.Done()
		w.QueueCommand(app.Stop)
	}()

	backend.Run(w)
	return nil
}
