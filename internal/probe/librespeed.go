package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Server is one LibreSpeed backend: an origin, the four paths hung off
// it, and where in the world it is.
//
// The paths are per-server because the public list disagrees about
// them. Some entries point the origin straight at the backend directory
// and name "garbage.php"; others give a bare origin and name
// "backend/garbage.php". Carrying both halves is simpler than guessing.
type Server struct {
	// Name is what the picker shows, as the published list writes it.
	Name string
	// City is the short label for the map pin.
	City string
	// Lat and Lng place that pin. See libreServers for why these are
	// hand-written rather than fetched.
	Lat, Lng float64
	// URL is the origin. Paths below are relative to it.
	URL string
	// Download, Upload, Ping and Info are the four endpoint paths.
	Download, Upload, Ping, Info string
}

// libreServers is a snapshot of https://librespeed.org/backend-servers/
// taken on 2026-09-03.
//
// Embedded rather than fetched for two reasons. The picker can show the
// list the instant the window opens, with no spinner and no failure
// mode before the user has asked for anything; and the published list
// carries no coordinates, so the map pin needs a hand-written table
// either way. The same trade-off colo.go already makes: the table is a
// snapshot, an entry that goes away is a failed run rather than a
// crash, and a demo app does not need to track a moving list.
var libreServers = []Server{
	{
		Name: "Amsterdam, Netherlands (Clouvider)", City: "Amsterdam",
		Lat: 52.3676, Lng: 4.9041,
		URL:      "https://ams.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Amsterdam, Netherlands (Sharktech)", City: "Amsterdam",
		Lat: 52.3676, Lng: 4.9041,
		URL:      "https://amsspeed.sharktech.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Argalasti, Greece (Cosmote)", City: "Argalasti",
		Lat: 39.2264, Lng: 23.2206,
		URL:      "https://argalasti.skoultsos.eu/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Atlanta, United States (Clouvider)", City: "Atlanta",
		Lat: 33.7490, Lng: -84.3880,
		URL:      "https://atl.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Belgrade, Serbia (SOX)", City: "Belgrade",
		Lat: 44.7866, Lng: 20.4489,
		URL:      "https://speedtest1.sox.rs/librespeed/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Chicago, United States (Sharktech)", City: "Chicago",
		Lat: 41.8781, Lng: -87.6298,
		URL:      "https://chispeed.sharktech.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Denver, United States (Sharktech)", City: "Denver",
		Lat: 39.7392, Lng: -104.9903,
		URL:      "https://denspeed.sharktech.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Frankfurt, Germany (Clouvider)", City: "Frankfurt",
		Lat: 50.1109, Lng: 8.6821,
		URL:      "https://fra.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Frankfurt, Germany (FS IT-Systeme)", City: "Frankfurt",
		Lat: 50.1109, Lng: 8.6821,
		URL:      "https://speed.fs-it.systems/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Grand Rapids, United States (RackGenius)", City: "Grand Rapids",
		Lat: 42.9634, Lng: -85.6681,
		URL:      "https://mispeed.rackgenius.com/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Helsinki, Finland (Hetzner)", City: "Helsinki",
		Lat: 60.1699, Lng: 24.9384,
		URL:      "https://www.librespeed.fi/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Las Vegas, United States (Sharktech)", City: "Las Vegas",
		Lat: 36.1699, Lng: -115.1398,
		URL:      "https://lasspeed.sharktech.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "London, England (Clouvider)", City: "London",
		Lat: 51.5074, Lng: -0.1278,
		URL:      "https://lon.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Los Angeles, United States (Clouvider)", City: "Los Angeles",
		Lat: 34.0522, Lng: -118.2437,
		URL:      "https://la.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Los Angeles, United States (Sharktech)", City: "Los Angeles",
		Lat: 34.0522, Lng: -118.2437,
		URL:      "https://laxspeed.sharktech.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "New York, United States (Clouvider)", City: "New York",
		Lat: 40.7128, Lng: -74.0060,
		URL:      "https://nyc.speedtest.clouvider.net/backend",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Novi Sad, Serbia (E-CAPS.net)", City: "Novi Sad",
		Lat: 45.2671, Lng: 19.8335,
		URL:      "https://speed1.e-caps.net",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Poznan, Poland (INEA)", City: "Poznan",
		Lat: 52.4064, Lng: 16.9252,
		URL:      "https://speedtest.kamilszczepanski.com",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Prague, Czech Republic (CESNET)", City: "Prague",
		Lat: 50.0755, Lng: 14.4378,
		URL:      "https://speedtest.cesnet.cz",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Prague, Czech Republic (Turris)", City: "Prague",
		Lat: 50.0755, Lng: 14.4378,
		URL:      "https://librespeed.turris.cz",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
	{
		Name: "Rome, Italy (GARR)", City: "Rome",
		Lat: 41.9028, Lng: 12.4964,
		URL:      "https://st-be-rm2.infra.garr.it",
		Download: "garbage.php", Upload: "empty.php",
		Ping: "empty.php", Info: "getIP.php",
	},
	{
		Name: "Tokyo, Japan (A573)", City: "Tokyo",
		Lat: 35.6762, Lng: 139.6503,
		URL:      "https://librespeed.a573.net/",
		Download: "backend/garbage.php", Upload: "backend/empty.php",
		Ping: "backend/empty.php", Info: "backend/getIP.php",
	},
}

// LibreServers returns the embedded server list.
//
// A copy, for the same reason Providers is: the caller is a UI holding
// the slice across frames.
func LibreServers() []Server {
	out := make([]Server, len(libreServers))
	copy(out, libreServers)
	return out
}

// libreSpeedBackend speaks the LibreSpeed backend protocol.
type libreSpeedBackend struct{}

// mib is the unit garbage.php counts in.
const mib = 1 << 20

// maxCkSize is garbage.php's own ceiling. Asking for more is refused
// upstream, so the ladder is clamped here instead.
const maxCkSize = 1024

// downloadURL asks garbage.php for a payload.
//
// The size is given in whole mebibytes, which is the only unit the
// endpoint takes. The ladder's first rung is 100 KB and therefore
// rounds up to one MiB: the warm-up stage is a little larger against
// LibreSpeed than against Cloudflare. That costs a fraction of a second
// at the start of the phase and nothing after it, because
// MinPhaseDuration decides when the phase actually ends.
func (libreSpeedBackend) downloadURL(cfg Config, size int64) string {
	ck := min(max((size+mib-1)/mib, 1), maxCkSize)
	return joinPath(cfg.Server.URL, cfg.Server.Download) +
		"?ckSize=" + strconv.FormatInt(ck, 10) + "&r=" + bust()
}

func (libreSpeedBackend) uploadURL(cfg Config) string {
	return joinPath(cfg.Server.URL, cfg.Server.Upload) + "?r=" + bust()
}

func (libreSpeedBackend) pingURL(cfg Config) string {
	return joinPath(cfg.Server.URL, cfg.Server.Ping) + "?r=" + bust()
}

// infoLimit caps the getIP response. The real one is a few hundred
// bytes; anything larger is a captive portal, not the JSON we asked
// for.
const infoLimit = 16 << 10

// libreInfo is getIP.php's response.
//
// rawIspInfo is deliberately a RawMessage. Servers without the IP
// database return null for it, and some LibreSpeed versions return an
// empty string, so decoding it straight into a struct would fail on
// well-behaved servers. It is decoded separately and its absence is
// normal.
type libreInfo struct {
	ProcessedString string          `json:"processedString"`
	RawIspInfo      json.RawMessage `json:"rawIspInfo"`
}

// libreIspInfo is the subset of the IP database record this app reads.
// Field names cover both shapes the servers return: the current "lite"
// record, which carries a country but no coordinates, and the older
// full record, which carries "loc" as "lat,lng".
type libreIspInfo struct {
	ASN     string `json:"asn"`
	ASName  string `json:"as_name"`
	Org     string `json:"org"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Loc     string `json:"loc"`
}

// identify asks the server who is calling, and reports where the server
// itself is from the embedded table.
//
// LibreSpeed has no equivalent of Cloudflare's /meta: there is no
// coordinate for the client on most servers, and none at all for the
// server. So the far-end pin comes from cfg.Server, and the client pin
// falls back to the same country centroid the Cloudflare path uses.
// When neither is known the map draws the one pin it has, which
// applyTrace already handles.
func (libreSpeedBackend) identify(ctx context.Context, cfg Config) (*Trace, error) {
	t := &Trace{
		// Colo is left empty on purpose. It holds a datacenter code,
		// and this protocol has none; filling it with the city would
		// make every label read "Amsterdam (Amsterdam)". The display
		// sites all fall back to ColoCity when it is empty.
		ColoCity:  clip(cfg.Server.City, 64),
		ColoLat:   cfg.Server.Lat,
		ColoLng:   cfg.Server.Lng,
		ColoKnown: cfg.Server.Lat != 0 || cfg.Server.Lng != 0,
	}

	url := joinPath(cfg.Server.URL, cfg.Server.Info) + "?isp=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server info request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server info: unexpected status %s", resp.Status)
	}

	// Read with limit+1 so an oversized body is rejected rather than
	// silently truncated into something that still decodes.
	data, err := io.ReadAll(io.LimitReader(resp.Body, infoLimit+1))
	if err != nil {
		return nil, fmt.Errorf("server info read: %w", err)
	}
	if len(data) > infoLimit {
		return nil, fmt.Errorf("server info: response too large (%d bytes)", len(data))
	}
	var info libreInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("server info decode: %w", err)
	}

	applyLibreInfo(t, &info)
	// Only the client half is looked up: the server half is already
	// set above and resolveLocations would not recognise a city name
	// as a datacenter code anyway.
	if c, ok := countryCentroids[t.Loc]; ok {
		t.ClientLat = c[0]
		t.ClientLng = c[1]
		t.ClientKnown = true
	}
	return t, nil
}

// applyLibreInfo copies what the response carried onto the trace.
//
// Everything here is best effort. The protocol guarantees only the IP,
// and four of the six servers checked while this was written returned
// nothing else, so each field is filled if present and skipped if not.
func applyLibreInfo(t *Trace, info *libreInfo) {
	if t == nil || info == nil {
		return
	}
	// processedString is "<ip>", "<ip> - <isp>", or
	// "<ip> - <isp>, <country name>". The IP is always the first field.
	ip, rest, _ := strings.Cut(info.ProcessedString, " - ")
	t.IP = clip(ip, 64)
	if org, _, _ := strings.Cut(rest, ","); org != "" && org != "Unknown ISP" {
		t.ASOrg = clip(org, 96)
	}

	var raw libreIspInfo
	// A null or a string here is the normal "no IP database" answer,
	// not a broken server, so the error is dropped.
	if json.Unmarshal(info.RawIspInfo, &raw) != nil {
		return
	}
	if raw.ASName != "" {
		t.ASOrg = clip(raw.ASName, 96)
	} else if raw.Org != "" {
		t.ASOrg = clip(raw.Org, 96)
	}
	// The record spells the number "AS394147".
	if n, err := strconv.ParseUint(
		strings.TrimPrefix(strings.ToUpper(raw.ASN), "AS"), 10, 32,
	); err == nil {
		t.ASN = int(n)
	}
	t.City = clip(raw.City, 64)
	t.Region = clip(raw.Region, 64)
	if raw.Country != "" {
		t.Loc = clip(strings.ToUpper(raw.Country), 16)
	}
	// The older full record carries real coordinates. When it does they
	// beat the country centroid the caller would otherwise fall back
	// to, so they are applied here and the flag is set.
	if lat, lng, ok := parseLatLng(raw.Loc); ok {
		t.ClientLat, t.ClientLng = lat, lng
		t.ClientKnown = true
	}
}

// parseLatLng reads the "lat,lng" pair the full IP record uses, and
// rejects anything off the globe.
func parseLatLng(s string) (lat, lng float64, ok bool) {
	a, b, found := strings.Cut(s, ",")
	if !found {
		return 0, 0, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(a), 64)
	if err != nil {
		return 0, 0, false
	}
	lng, err = strconv.ParseFloat(strings.TrimSpace(b), 64)
	if err != nil {
		return 0, 0, false
	}
	// ParseFloat accepts NaN and Inf by name; the range check rejects
	// both, so no separate math.IsNaN test is needed.
	if !(lat >= -90 && lat <= 90) || !(lng >= -180 && lng <= 180) {
		return 0, 0, false
	}
	return lat, lng, true
}
