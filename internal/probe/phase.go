package probe

// Phase names one stage of a speed test run. The UI keys almost all of
// its presentation off the current phase: which chart is live, whether
// the mascot spins, what the map link line shows.
type Phase int

const (
	// PhaseIdle is the state before Run starts and is never emitted by
	// the engine itself.
	PhaseIdle Phase = iota
	// PhaseTrace resolves the serving Cloudflare datacenter and the
	// client's own approximate location. It feeds the map.
	PhaseTrace
	// PhaseLatency collects round-trip times with tiny requests. It
	// feeds the box plot and the histogram.
	PhaseLatency
	// PhaseDownload measures throughput pulling staged payload sizes.
	PhaseDownload
	// PhaseUpload measures throughput pushing staged payload sizes.
	PhaseUpload
	// PhaseDone marks a completed run. Charts freeze at their last
	// values.
	PhaseDone
	// PhaseError marks a run abandoned because a stage failed. The
	// engine stops; whatever was already measured stays on screen.
	PhaseError
)

// String returns a short lower-case name, suitable for logs and for the
// -once CLI report.
func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseTrace:
		return "trace"
	case PhaseLatency:
		return "latency"
	case PhaseDownload:
		return "download"
	case PhaseUpload:
		return "upload"
	case PhaseDone:
		return "done"
	case PhaseError:
		return "error"
	}
	return "unknown"
}

// Label returns a title-case name for on-screen use.
func (p Phase) Label() string {
	switch p {
	case PhaseIdle:
		return "Ready"
	case PhaseTrace:
		return "Locating"
	case PhaseLatency:
		return "Latency"
	case PhaseDownload:
		return "Download"
	case PhaseUpload:
		return "Upload"
	case PhaseDone:
		return "Complete"
	case PhaseError:
		return "Failed"
	}
	return "Unknown"
}

// Active reports whether a run is in progress. The mascot spins while
// this is true.
func (p Phase) Active() bool {
	return p == PhaseTrace || p == PhaseLatency || p == PhaseDownload || p == PhaseUpload
}
