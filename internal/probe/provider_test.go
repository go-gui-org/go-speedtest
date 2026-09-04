package probe

import (
	"strings"
	"testing"
)

func TestProviderByName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Demo (offline)", "Demo (offline)", true},
		{"demo", "Demo (offline)", true},
		{"CLOUD", "Cloudflare", true},
		{"libre", "LibreSpeed", true},
		{"custom", "Custom URL", true},
		{"  demo  ", "Demo (offline)", true},
		{"", "", false},
		{"nonesuch", "", false},
		// A prefix, not a substring: matching "flare" would make the
		// flag accept names nobody would guess were the same entry.
		{"flare", "", false},
	}
	for _, c := range cases {
		p, _, ok := ProviderByName(c.in)
		if ok != c.ok || p.Name != c.want {
			t.Errorf("ProviderByName(%q) = %q,%v want %q,%v",
				c.in, p.Name, ok, c.want, c.ok)
		}
	}
}

func TestProvidersIsACopy(t *testing.T) {
	a := Providers()
	a[0].Name = "clobbered"
	if Providers()[0].Name == "clobbered" {
		t.Fatal("Providers() handed out the package's own slice")
	}
}

func TestProviderApply(t *testing.T) {
	demo, _, _ := ProviderByName("demo")
	if got := demo.Apply(Config{}, "ignored", 0); !got.Simulate || got.BaseURL != "" {
		t.Errorf("demo applied to %+v", got)
	}

	cf, _, _ := ProviderByName("cloud")
	got := cf.Apply(Config{}, "ignored", 0)
	if got.BaseURL != DefaultBaseURL || got.Simulate ||
		got.Backend != BackendCloudflare {
		t.Errorf("cloudflare applied to %+v", got)
	}

	custom, _, _ := ProviderByName("custom")
	if got := custom.Apply(Config{}, "  https://h.example  ", 0); got.BaseURL != "https://h.example" {
		t.Errorf("custom applied to %+v", got)
	}

	// LibreSpeed carries its origin on the server, not on BaseURL, and
	// the index picks which one.
	ls, _, _ := ProviderByName("libre")
	got = ls.Apply(Config{}, "ignored", 1)
	if got.Backend != BackendLibreSpeed {
		t.Errorf("librespeed backend = %v", got.Backend)
	}
	if got.Server.Name != ls.Servers[1].Name {
		t.Errorf("librespeed server = %q, want %q",
			got.Server.Name, ls.Servers[1].Name)
	}
	// A stale index from the UI must fall back rather than panic.
	if got := ls.Apply(Config{}, "", 9999); got.Server.Name != ls.Servers[0].Name {
		t.Errorf("out-of-range index gave %q", got.Server.Name)
	}
}

func TestSelectionResolve(t *testing.T) {
	// An empty provider name means the default, not an error: the flag
	// is optional and the report mode calls this unconditionally.
	p, pi, si, err := Selection{}.Resolve()
	if err != nil || pi != DefaultProvider || si != 0 || p.Name != "Cloudflare" {
		t.Fatalf("empty selection = %q,%d,%d,%v", p.Name, pi, si, err)
	}

	// A server name is matched on any part of the label, unlike a
	// provider name.
	p, _, si, err = Selection{Provider: "libre", Server: "tokyo"}.Resolve()
	if err != nil {
		t.Fatalf("librespeed/tokyo = %v", err)
	}
	if !strings.Contains(strings.ToLower(p.Servers[si].Name), "tokyo") {
		t.Errorf("server %d is %q, want a Tokyo one", si, p.Servers[si].Name)
	}

	bad := []Selection{
		{Provider: "nonesuch"},
		{Provider: "custom"},                         // no URL
		{Provider: "custom", CustomURL: "not a url"}, // unusable URL
		{Provider: "cloud", Server: "tokyo"},         // no server list
		{Provider: "libre", Server: "atlantis"},      // no such server
	}
	for _, sel := range bad {
		if _, _, _, err := sel.Resolve(); err == nil {
			t.Errorf("Resolve(%+v) = nil, want an error", sel)
		}
	}
}

func TestLibreServersAreUsable(t *testing.T) {
	list := LibreServers()
	if len(list) < 10 {
		t.Fatalf("only %d LibreSpeed servers", len(list))
	}
	seen := map[string]bool{}
	for _, srv := range list {
		if seen[srv.Name] {
			t.Errorf("duplicate server name %q", srv.Name)
		}
		seen[srv.Name] = true
		// Every field is used to build a request or a map pin, so an
		// empty one is a typo in the table rather than a missing
		// feature.
		if srv.URL == "" || srv.Download == "" || srv.Upload == "" ||
			srv.Ping == "" || srv.Info == "" || srv.City == "" {
			t.Errorf("server %q has an empty field: %+v", srv.Name, srv)
		}
		if !strings.HasPrefix(srv.URL, "https://") {
			t.Errorf("server %q is not https: %q", srv.Name, srv.URL)
		}
		if srv.Lat < -90 || srv.Lat > 90 || srv.Lng < -180 || srv.Lng > 180 {
			t.Errorf("server %q is off the globe: %v,%v", srv.Name, srv.Lat, srv.Lng)
		}
	}

	list[0].Name = "clobbered"
	if LibreServers()[0].Name == "clobbered" {
		t.Fatal("LibreServers() handed out the package's own slice")
	}
}

func TestCheckURL(t *testing.T) {
	good := []string{
		"https://speed.cloudflare.com",
		"http://192.0.2.1:8080",
		"https://host.example/prefix",
	}
	for _, u := range good {
		if err := CheckURL(u); err != nil {
			t.Errorf("CheckURL(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{
		"",
		"   ",
		"speed.cloudflare.com",   // no scheme
		"ftp://host.example",     // wrong scheme
		"https://",               // no host
		"https://h.example?a=1",  // query would land mid-path
		"https://h.example#frag", // same for a fragment
	}
	for _, u := range bad {
		if err := CheckURL(u); err == nil {
			t.Errorf("CheckURL(%q) = nil, want an error", u)
		}
	}
}
