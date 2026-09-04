package probe

import (
	"fmt"
	"net/url"
	"strings"
)

// Provider is one place a run can be pointed at.
//
// A provider is mostly an origin, because two of the three real entries
// speak Cloudflare's speed worker — /__down?bytes=N, /__up,
// /cdn-cgi/trace and /meta — and differ only in which host runs it.
// LibreSpeed does not: different paths, a different way of asking for a
// payload size, and a different endpoint for who is at each end. That
// is what Backend selects, and what Servers carries the addresses for.
type Provider struct {
	// Name is what the picker shows.
	Name string
	// Backend is the protocol this provider speaks.
	Backend Backend
	// BaseURL is the origin the endpoints hang off, for the backends
	// that use one. Empty for the simulated provider, which makes no
	// requests; for the custom entry, whose URL comes from the user;
	// and for LibreSpeed, whose origin comes from the chosen server.
	BaseURL string
	// Servers is the list of hosts to choose between, for a provider
	// that publishes one. Empty means the provider is a single origin
	// and the UI shows no server picker.
	Servers []Server
	// Simulate runs the offline generator instead of the network.
	Simulate bool
	// Custom marks the entry the user supplies a URL for. Exactly one
	// entry has it set, and the UI reveals a text field when it is
	// selected.
	Custom bool
}

// providers is the fixed list, in the order the picker shows it.
//
// Demo is first because it is the one entry that touches nothing, so it
// is the safe thing to land on and the right thing to try first.
var providers = []Provider{
	{
		Name:     "Demo (offline)",
		Simulate: true,
	},
	{
		Name:    "Cloudflare",
		Backend: BackendCloudflare,
		BaseURL: DefaultBaseURL,
	},
	{
		Name:    "LibreSpeed",
		Backend: BackendLibreSpeed,
		Servers: libreServers,
	},
	{
		Name:   "Custom URL",
		Custom: true,
	},
}

// Providers returns the selectable providers.
//
// A copy, because the caller is a UI that holds the slice across
// frames, and a shared backing array is a bug waiting for the day
// somebody sorts it.
func Providers() []Provider {
	out := make([]Provider, len(providers))
	copy(out, providers)
	return out
}

// ProviderByName looks an entry up by the name the picker shows.
//
// Matched case-insensitively and on a prefix, so the command line can
// say "demo" or "cloud" rather than "Demo (offline)".
func ProviderByName(name string) (Provider, int, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return Provider{}, 0, false
	}
	for i, p := range providers {
		if strings.HasPrefix(strings.ToLower(p.Name), want) {
			return p, i, true
		}
	}
	return Provider{}, 0, false
}

// ProviderNames lists the entries, for a flag's error message.
func ProviderNames() []string {
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p.Name
	}
	return out
}

// DefaultProvider is the index Providers() starts on for a normal run:
// the real endpoint, not the generator.
const DefaultProvider = 1

// DemoProvider is the index of the offline entry, which is what the
// -demo flag selects.
const DemoProvider = 0

// Apply writes the provider's contribution into cfg and returns it.
//
// One function rather than a handful of getters because the fields move
// together: a provider decides the protocol, the origin and the server
// as one choice, and letting a caller pick up two of the three is how
// a LibreSpeed run ends up pointed at Cloudflare's origin.
//
// custom is only read for the custom entry, and only after CheckURL has
// passed on it — an unusable URL would otherwise reach the engine and
// fail three phases later with a connection error. serverIdx is only
// read for a provider that has servers, and is clamped, so a stale
// index from the UI selects the first server rather than panicking.
func (p Provider) Apply(cfg Config, custom string, serverIdx int) Config {
	cfg.Backend = p.Backend
	switch {
	case p.Simulate:
		cfg.Simulate = true
		return cfg
	case p.Custom:
		cfg.BaseURL = strings.TrimSpace(custom)
	default:
		cfg.BaseURL = p.BaseURL
	}
	if len(p.Servers) > 0 {
		cfg.Server = p.Servers[ClampServer(p, serverIdx)]
	}
	// How hard a run may push is a property of the host, so it is
	// settled here alongside the address rather than in the engine.
	return cfg.Backend.tune(cfg)
}

// ClampServer bounds a server index to the provider's list, returning 0
// for a provider that has no servers.
func ClampServer(p Provider, idx int) int {
	if idx < 0 || idx >= len(p.Servers) {
		return 0
	}
	return idx
}

// ServerByName finds one of a provider's servers by a case-insensitive
// substring of its name, so the command line can say "tokyo" rather
// than the whole published label.
//
// Substring here, unlike the prefix match ProviderByName uses, because
// a server name leads with its city and the interesting word is often
// the sponsor at the end.
func ServerByName(p Provider, name string) (int, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return 0, false
	}
	for i, s := range p.Servers {
		if strings.Contains(strings.ToLower(s.Name), want) {
			return i, true
		}
	}
	return 0, false
}

// Selection is a provider chosen by name, the way a command line
// spells it: which provider, the URL for the custom entry, and which
// server for a provider that has a list.
//
// It exists so the window and the headless modes share one resolver.
// Two copies of "what does this name mean" is how a flag and a picker
// end up disagreeing about which host was measured.
type Selection struct {
	// Provider is a prefix of a provider name, case-insensitive. Empty
	// means the default provider.
	Provider string
	// CustomURL is the base URL for the custom entry. Ignored, but
	// still carried, for every other entry.
	CustomURL string
	// Server is a substring of a server name. Empty means the first
	// server, and naming one for a provider with no list is an error.
	Server string
}

// Resolve validates a Selection and returns the entry it names along
// with the two indices the UI holds.
//
// Everything that can be wrong with a selection is reported here, so a
// bad name given on the command line is a startup error rather than a
// failure eight seconds into a run.
func (sel Selection) Resolve() (p Provider, providerIdx, serverIdx int, err error) {
	if strings.TrimSpace(sel.Provider) == "" {
		providerIdx = DefaultProvider
		p = providers[providerIdx]
	} else {
		var ok bool
		p, providerIdx, ok = ProviderByName(sel.Provider)
		if !ok {
			return Provider{}, 0, 0, fmt.Errorf(
				"unknown provider %q, want one of %s",
				sel.Provider, strings.Join(ProviderNames(), ", "))
		}
	}

	if p.Custom {
		if err := CheckURL(sel.CustomURL); err != nil {
			return Provider{}, 0, 0, fmt.Errorf("provider %s: %w", p.Name, err)
		}
	}

	// A server name is only meaningful for a provider that has a list.
	// Naming one for a provider that does not is a typo worth
	// reporting, not something to ignore.
	if name := strings.TrimSpace(sel.Server); name != "" {
		if len(p.Servers) == 0 {
			return Provider{}, 0, 0, fmt.Errorf(
				"provider %s has no server list", p.Name)
		}
		var ok bool
		serverIdx, ok = ServerByName(p, name)
		if !ok {
			return Provider{}, 0, 0, fmt.Errorf(
				"unknown %s server %q, want one of %s",
				p.Name, name, strings.Join(ServerNames(p), ", "))
		}
	}
	return p, providerIdx, serverIdx, nil
}

// ServerNames lists a provider's servers, for a picker or a flag's
// error message.
func ServerNames(p Provider) []string {
	out := make([]string, len(p.Servers))
	for i, s := range p.Servers {
		out[i] = s.Name
	}
	return out
}

// CheckURL reports what is wrong with a custom provider URL, or nil.
//
// Checked here rather than at the point of use so the picker can refuse
// to start a run and say why, instead of starting one that cannot work.
// The rules are narrow on purpose: this URL becomes the prefix of every
// request the engine makes.
func CheckURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errBadURL("enter a base URL, e.g. https://speed.cloudflare.com")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errBadURL("not a URL: " + err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errBadURL("needs an http:// or https:// scheme")
	}
	if u.Host == "" {
		return errBadURL("no host in the URL")
	}
	// A query or a fragment would end up in the middle of the request
	// path once the endpoint is appended, which fails in a way that
	// looks like the provider being down.
	if u.RawQuery != "" || u.Fragment != "" {
		return errBadURL("drop the query and fragment: this is a base URL")
	}
	return nil
}

// errBadURL is the error CheckURL returns. A distinct type so the UI can
// show the message without a wrapping prefix.
type errBadURL string

func (e errBadURL) Error() string { return string(e) }
