package main

import (
	"runtime/debug"
	"strings"
)

// version is the build version reported by -version. The Makefile and
// the release workflow stamp it at link time from `git describe --tags`,
// so a tagged build reports its tag verbatim and an in-between build
// reports "v0.2.0-3-g1a2b3c4".
//
// Left empty rather than carrying a literal: a hand-maintained constant
// goes stale silently, and a wrong version is worse than an honest
// fallback.
var version string

// appVersion resolves the version to show. Preference order:
//
//  1. the linker-stamped tag, which the shipping path always sets;
//  2. the module version, set when the binary came from
//     `go install github.com/go-gui-org/go-speedtest/...@v0.2.0`;
//  3. the VCS revision the toolchain embeds in any build inside a git
//     checkout;
//  4. "dev", for `go run` and -buildvcs=false builds, where nothing is
//     known.
func appVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	// "(devel)" is what the toolchain reports for a locally built main
	// module. That is not a version, so fall through to the VCS stamps.
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return vcsVersion(info)
}

// vcsVersion builds a short revision string from the vcs.* settings the
// toolchain embeds. Returns "dev" when the build carried none: a module
// cache build, a source tarball, or -buildvcs=false.
func vcsVersion(info *debug.BuildInfo) string {
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	// Abbreviate to the usual 7-character short hash.
	if len(rev) > 7 {
		rev = rev[:7]
	}
	var b strings.Builder
	b.WriteString("dev-")
	b.WriteString(rev)
	if dirty {
		b.WriteString("-dirty")
	}
	return b.String()
}
