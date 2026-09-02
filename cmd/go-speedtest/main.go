// Command go-speedtest measures network performance and shows the
// result as a live dashboard.
//
// It is also the flagship demo for three libraries working together:
// go-gui for the window, layout and animation, go-charts for the live
// area charts, box plot, histogram and gauge, and go-map for the
// datacenter map.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"
)

func main() {
	var (
		demo    = flag.Bool("demo", false, "run the offline generator instead of touching the network")
		once    = flag.Bool("once", false, "run one test, print a text report and exit")
		start   = flag.Bool("start", false, "begin a run as soon as the window opens")
		timeout = flag.Duration("timeout", 90*time.Second, "give up on a run after this long")
		shot    = flag.String("screenshot", "", "render one frame to this PNG path and exit")
		shotAt  = flag.Duration("screenshot-at", 9*time.Second, "how far into the run to capture")
	)
	flag.Parse()

	// Ctrl-C stops a run in progress rather than killing the process
	// mid-transfer, so the CLI report still prints what it measured.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// No defer: every path below ends in os.Exit or in a blocking
	// backend loop, and a deferred stop would not run in either case.
	// The signal handler lives as long as the process, which is what
	// this program wants anyway.
	_ = stop

	if *shot != "" {
		if err := shoot(*shot, *demo, *shotAt, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "go-speedtest:", err)
			os.Exit(1)
		}
		return
	}

	if *once {
		if err := runOnce(ctx, *demo, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "go-speedtest:", err)
			os.Exit(1)
		}
		return
	}

	if err := runGUI(ctx, *demo, *timeout, *start); err != nil {
		fmt.Fprintln(os.Stderr, "go-speedtest:", err)
		os.Exit(1)
	}
}
