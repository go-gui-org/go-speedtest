package probe

import "time"

// EventKind tells the consumer which fields of an Event are populated.
// One struct with a kind tag beats a sum type here: events cross a
// channel to the UI goroutine, and a single concrete type keeps that
// hand-off allocation-free once the channel buffer is warm.
type EventKind int

const (
	// EventPhase reports that the engine moved to Event.Phase. Sent
	// before any measurement of that phase.
	EventPhase EventKind = iota
	// EventTrace carries the resolved datacenter and client location in
	// Event.Trace.
	EventTrace
	// EventRTT carries one round-trip sample in Event.RTT.
	EventRTT
	// EventRate carries an instantaneous throughput reading in
	// Event.Mbps, measured over the last chunk. Event.Elapsed is time
	// since the phase started, which is the chart's X axis.
	EventRate
	// EventStage reports a finished payload stage. Event.Mbps is the
	// mean rate over that whole stage, Event.Bytes its size.
	EventStage
	// EventError carries a fatal error in Event.Err. The engine stops
	// after sending it.
	EventError
	// EventWarning carries a non-fatal error in Event.Err. The phase
	// it names ended early, but the run continues and whatever that
	// phase already measured stays valid.
	EventWarning
	// EventDone carries the completed Result. Always the last event.
	EventDone
)

// Event is a single update from the engine to the UI. Exactly one run
// of the engine produces a stream of these, ending in EventDone or
// EventError.
type Event struct {
	Kind  EventKind
	Phase Phase

	// Elapsed is measured from the start of the current phase, not from
	// the start of the run, so each chart's X axis starts at zero.
	Elapsed time.Duration

	Trace *Trace        // EventTrace
	RTT   time.Duration // EventRTT
	Mbps  float64       // EventRate, EventStage
	Bytes int64         // EventStage
	Err   error         // EventError, EventWarning
	Res   *Result       // EventDone
}

// Trace is what the Cloudflare trace endpoint tells us about this
// connection: which datacenter answered, and roughly where we are.
type Trace struct {
	// Colo is the three-letter Cloudflare datacenter code, for example
	// "SEA". Empty if the endpoint did not report one.
	Colo string
	// ColoCity is the human name for Colo, resolved from an embedded
	// table. Empty when the code is unknown to us.
	ColoCity string
	// ColoLat and ColoLng place the datacenter pin. Valid only when
	// ColoKnown is true.
	ColoLat, ColoLng float64
	ColoKnown        bool

	// IP is the client's public address as the edge saw it.
	IP string
	// Loc is the two-letter country code of the client.
	Loc string
	// ClientLat and ClientLng place the client pin, resolved from Loc
	// to a country centroid. Valid only when ClientKnown is true. This
	// is deliberately coarse: no geo-IP lookup, no third-party service.
	ClientLat, ClientLng float64
	ClientKnown          bool

	// ASN and ASOrg name the network the client is connected through,
	// as the edge sees it. Zero and empty when the meta call did not
	// answer, which is not an error: see fetchMeta.
	ASN   int
	ASOrg string

	// City and Region are the client's approximate location, reported
	// by the edge rather than looked up here. Empty when unknown.
	City, Region string

	// HTTPProtocol is what the transfers actually negotiated, for
	// example "HTTP/2". A run over HTTP/1.1 measures a different thing
	// from a run over HTTP/3, so it is worth showing.
	HTTPProtocol string
}

// Result is the summary of a finished run. The UI keeps it for the
// final stat row; the CLI mode prints it.
type Result struct {
	Trace Trace

	// RTTs holds every latency sample in collection order. The box plot
	// and histogram take this slice raw.
	RTTs []time.Duration

	// DownMbps and UpMbps are the headline numbers: the 90th percentile
	// of instantaneous readings, which is closer to what a user
	// experiences than a mean dragged down by TCP ramp-up.
	DownMbps float64
	UpMbps   float64

	// DownBytes and UpBytes are the totals actually transferred.
	DownBytes int64
	UpBytes   int64

	// Duration is wall-clock time for the whole run.
	Duration time.Duration

	// Simulated is true when the numbers came from the offline demo
	// generator rather than the network.
	Simulated bool
}
