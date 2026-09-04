package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/go-gui-org/go-speedtest/internal/format"
	"github.com/go-gui-org/go-speedtest/internal/probe"
	"github.com/go-gui-org/go-speedtest/internal/stats"
)

// runOnce drives the engine with no window and prints a report.
//
// This mode exists to keep the measurement honest: it exercises exactly
// the same engine the dashboard does, so a number that looks wrong on
// screen can be checked without a GUI in the way.
func runOnce(ctx context.Context, sel probe.Selection,
	timeout time.Duration,
) error {
	// Same resolver the window's picker uses, so the report mode and
	// the dashboard cannot disagree about what a provider name means.
	p, _, serverIdx, err := sel.Resolve()
	if err != nil {
		return err
	}
	eng := probe.New(p.Apply(probe.Config{Timeout: timeout}, sel.CustomURL, serverIdx))

	var (
		res      *probe.Result
		runErr   error
		lastLine string
	)
	for ev := range eng.Run(ctx) {
		switch ev.Kind {
		case probe.EventPhase:
			if ev.Phase.Active() {
				lastLine = "  " + ev.Phase.Label() + "..."
				_, _ = fmt.Fprint(os.Stderr, "\r"+lastLine)
			}
		case probe.EventRate:
			// Overwrite one line rather than scrolling, so piping the
			// report to a file leaves the progress noise on stderr.
			_, _ = fmt.Fprintf(os.Stderr, "\r  %-9s %8s Mbps",
				ev.Phase.Label(), format.Mbps(ev.Mbps))
		case probe.EventError:
			runErr = ev.Err
		case probe.EventDone:
			res = ev.Res
		}
	}
	_, _ = fmt.Fprint(os.Stderr, "\r\033[K") // clear the progress line

	if runErr != nil {
		return runErr
	}
	if res == nil {
		if ctx.Err() != nil {
			return errors.New("interrupted")
		}
		return errors.New("run produced no result")
	}
	printReport(res)
	return nil
}

// printReport writes the summary table to stdout.
func printReport(res *probe.Result) {
	rtts := millis(res.RTTs)

	// A report is a page of text to a terminal. Checking each write
	// would triple the length of this function to handle a failure
	// that means stdout is gone, so errors are collected by the
	// tabwriter and reported once at the end.
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	line := func(f string, args ...any) {
		_, _ = fmt.Fprintf(tw, f, args...)
	}

	if res.Simulated {
		line("SIMULATED RUN\tno network was used\n")
	}
	line("Server\t%s\n", coloLine(res.Trace))
	if res.Trace.IP != "" {
		// The country is best effort: most LibreSpeed servers have no
		// IP database behind them, so the brackets are dropped rather
		// than printed empty.
		if res.Trace.Loc != "" {
			line("Client\t%s (%s)\n", res.Trace.IP, res.Trace.Loc)
		} else {
			line("Client\t%s\n", res.Trace.IP)
		}
	}
	line("\t\n")
	line("Download\t%s Mbps\t(%s transferred)\n",
		format.Mbps(res.DownMbps), format.Bytes(res.DownBytes))
	line("Upload\t%s Mbps\t(%s transferred)\n",
		format.Mbps(res.UpMbps), format.Bytes(res.UpBytes))
	line("\t\n")
	line("Latency min\t%s ms\n", format.MillisF(stats.Min(rtts)))
	line("Latency p50\t%s ms\n", format.MillisF(stats.Median(rtts)))
	line("Latency p95\t%s ms\n", format.MillisF(stats.Percentile(rtts, 0.95)))
	line("Jitter\t%s ms\t(mean absolute delta, RFC 3393)\n",
		format.MillisF(stats.Jitter(rtts)))
	line("Samples\t%d\n", len(rtts))
	line("\t\n")
	line("Elapsed\t%s\n", res.Duration.Round(100*time.Millisecond))

	if err := tw.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "go-speedtest: writing report:", err)
	}
}

// coloLine describes the answering host as far as we can resolve it.
//
// The bracketed code is Cloudflare's datacenter identifier; a
// LibreSpeed server has none and prints as its city alone. An
// unresolved code still prints, because the code itself is useful.
func coloLine(t probe.Trace) string {
	switch {
	case t.ColoCity != "" && t.Colo != "":
		return fmt.Sprintf("%s (%s)", t.ColoCity, t.Colo)
	case t.ColoCity != "":
		return t.ColoCity
	case t.Colo != "":
		return t.Colo
	default:
		return "unknown"
	}
}

// millis converts durations to float milliseconds for the stats
// helpers, keeping collection order so Jitter stays meaningful.
func millis(ds []time.Duration) []float64 {
	out := make([]float64, len(ds))
	for i, d := range ds {
		out[i] = float64(d) / float64(time.Millisecond)
	}
	return out
}
