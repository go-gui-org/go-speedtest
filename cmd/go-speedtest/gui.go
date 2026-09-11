package main

import (
	"context"
	"time"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-gui/gui/backend"
	"github.com/go-gui-org/go-map/tile"
	"github.com/go-gui-org/go-speedtest/internal/app"
	"github.com/go-gui-org/go-speedtest/internal/icon"
	"github.com/go-gui-org/go-speedtest/internal/probe"
)

// runGUI opens the dashboard and blocks until the window closes.
func runGUI(ctx context.Context, sel probe.Selection,
	timeout time.Duration, autostart bool,
) error {
	gui.SetTheme(gui.ThemeDark)

	src := app.TileSource()
	st := app.New(false, timeout, src)
	if err := st.SelectProvider(sel); err != nil {
		return err
	}

	cfg := gui.WindowCfg{
		State: st,
		Title: "go-speedtest",
		// Without this the backend installs go-gui's own default
		// icon over ours, in the Dock on macOS and in the taskbar
		// on Linux and Windows. See internal/icon.
		IconPNG: icon.AppPNG,
		Width:   1180,
		Height:  780,
		OnInit: func(w *gui.Window) {
			// Register the view once. Every later redraw goes through
			// InvalidateLayout, which keeps the map's viewport and the
			// charts' animation state.
			w.SetView(app.Root)
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
