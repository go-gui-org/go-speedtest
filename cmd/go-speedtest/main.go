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
	"strings"
	"time"

	"github.com/go-gui-org/go-speedtest/internal/probe"
)

func main() {
	var (
		demo = flag.Bool("demo", false, "run the offline generator instead of touching the network")
		// The picker in the window and this flag read the same list, so
		// a name that works in one works in the other. Prefix matching
		// means "demo" and "cloud" are enough.
		provider = flag.String("provider", "",
			"which provider to point at: "+strings.Join(probe.ProviderNames(), ", "))
		providerURL = flag.String("provider-url", "",
			"base URL for the custom provider, e.g. https://host.example")
		server = flag.String("server", "",
			"which server to use, for a provider that publishes a list "+
				"(LibreSpeed); matched on any part of the name, e.g. tokyo")
		once    = flag.Bool("once", false, "run one test, print a text report and exit")
		start   = flag.Bool("start", false, "begin a run as soon as the window opens")
		timeout = flag.Duration("timeout", 90*time.Second, "give up on a run after this long")
		shot    = flag.String("screenshot", "", "render one frame to this PNG path and exit")
		shotAt  = flag.Duration("screenshot-at", 9*time.Second, "how far into the run to capture")
		showVer = flag.Bool("version", false, "print the build version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("go-speedtest", appVersion())
		return
	}

	// -demo is the older spelling of "point at the offline generator".
	// Kept, and folded into the provider name here so there is one
	// selection path rather than two that can disagree.
	sel := probe.Selection{
		Provider:  *provider,
		CustomURL: *providerURL,
		Server:    *server,
	}
	if sel.Provider == "" && *demo {
		sel.Provider = "Demo"
	}

	// Ctrl-C stops a run in progress rather than killing the process
	// mid-transfer, so the CLI report still prints what it measured.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// No defer: every path below ends in os.Exit or in a blocking
	// backend loop, and a deferred stop would not run in either case.
	// The signal handler lives as long as the process, which is what
	// this program wants anyway.
	_ = stop

	if *shot != "" {
		if err := shoot(*shot, sel, *shotAt, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "go-speedtest:", err)
			os.Exit(1)
		}
		return
	}

	if *once {
		if err := runOnce(ctx, sel, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, "go-speedtest:", err)
			os.Exit(1)
		}
		return
	}

	if err := runGUI(ctx, sel, *timeout, *start); err != nil {
		fmt.Fprintln(os.Stderr, "go-speedtest:", err)
		os.Exit(1)
	}
}
