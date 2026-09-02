package main

import (
	"fmt"
	"os"
	"time"

	"github.com/go-gui-org/go-gui/gui"
	"github.com/go-gui-org/go-gui/gui/backend"
	"github.com/go-gui-org/go-gui/gui/backend/soft"
	"github.com/go-gui-org/go-map/tile"
	"github.com/go-gui-org/go-speedtest/internal/app"
)

// shoot renders the dashboard to a PNG and returns.
//
// It never starts the real backend. A run is driven to the requested
// point, then the software rasterizer draws one frame: no GPU, no
// window, and no race between the render loop and the sampler. This is
// how README images are made and how a phase can be inspected without
// racing the clock.
func shoot(path string, demo bool, at time.Duration, timeout time.Duration) error {
	gui.SetTheme(gui.ThemeDark)

	src := app.TileSource()
	st := app.New(demo, timeout, src)

	cfg := gui.WindowCfg{
		State:  st,
		Title:  "go-speedtest",
		Width:  1180,
		Height: 780,
		OnInit: func(w *gui.Window) {
			w.UpdateView(app.Root)
			app.Start(w)
		},
	}
	if f, ok := src.(tile.HTTPFetcher); ok {
		cfg.ImageFetcher = f.HTTPFetcher()
	}
	w := gui.NewWindow(cfg)

	// Capture from the window thread, through the command queue, while
	// the real frame loop is running. Rendering off-thread would race
	// the loop, and rendering without the loop leaves the view frozen
	// at whatever OnInit built: the refresh UpdateWindow requests is
	// serviced by the frame loop, not by the rasterizer.
	// A plain wait, not a poll on the run's state: this goroutine can
	// reach its first check before the window has finished starting,
	// and a "not running yet" reading would fire the capture at zero.
	go func() {
		time.Sleep(at)
		w.QueueCommand(func(w *gui.Window) {
			if err := soft.RenderToPNG(w, 2, path); err != nil {
				fmt.Fprintln(os.Stderr, "screenshot:", err)
				os.Exit(1)
			}
			fmt.Println("wrote", path)
			os.Exit(0)
		})
	}()

	backend.Run(w)
	return nil
}
