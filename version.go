package main

import (
	"runtime/debug"
	"strings"
)

// injected is the version stamped into release binaries, through
// `-ldflags "-X main.injected={{.Version}}"` in .goreleaser.yaml. Nothing else
// sets it: `go install` and a plain `go build` pass no ldflags at all, so in
// every binary that is not a goreleaser artefact it is the empty string, and
// resolveVersion falls back to the version the toolchain records.
//
// The name here and the one in .goreleaser.yaml have to match, and nothing
// enforces that at build time — `-X` naming a variable that does not exist is
// not an error, the linker drops it silently. A release would then report the
// fallback instead of the version it was cut as, with nothing to notice. That
// is what TestLdflagsVariableMatchesGoreleaser pins, so the declaration below
// is matched by a test as text: keep it as one `var injected string` line.
var injected string

// version is what `stressy --version` prints. It is resolved once, at package
// initialisation, and read by newCmd through the cobra command's Version
// field; Go orders package-level initialisation by dependency, so this is set
// before that literal is built.
var version = resolveVersion(injected, buildInfo())

// devVersion is reported when nothing in the binary knows what it is: built
// with -buildvcs=false, built outside version control, or built by a toolchain
// that recorded no build information. Reporting "devel" says that plainly,
// where the "0.0.0" placeholder this replaces looked like a real release
// version and was what every `go install` build printed (#40).
const devVersion = "devel"

// resolveVersion reports the version of this binary, preferring the value
// stamped at release time and falling back to the module version the toolchain
// records, which is populated on every build path:
//
//	build path                          Main.Version    reported
//	goreleaser release                  v0.4.0          0.4.0 (from injected)
//	go install ...@v0.4.0               v0.4.0          0.4.0
//	go build on a clean tagged tree     v0.4.0          0.4.0
//	go build on a dirty tree            v0.4.0+dirty    0.4.0+dirty
//	go build past the last tag          v0.4.1-0.2026…  0.4.1-0.2026…
//	go build -buildvcs=false            (devel)         devel
//	no build information at all         —               devel
//
// The leading "v" is stripped because goreleaser's {{ .Version }} has no
// prefix: without it the same release would print "0.4.0" or "v0.4.0"
// depending on which build path produced the binary. "+dirty" is kept — a
// binary built from a modified tree is not the release it is closest to, and
// that is worth seeing in a bug report.
//
// It takes both inputs as parameters rather than reading them itself so the
// whole table above is a test case rather than a build.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "" {
		return injected
	}

	// "(devel)" is what the toolchain records when it has no version to
	// record; passing it through would put a string in --version output that
	// no operator can match against a release.
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return devVersion
	}

	return strings.TrimPrefix(info.Main.Version, "v")
}

// buildInfo is debug.ReadBuildInfo with its second return value folded into
// the first: unavailable and empty are the same case here, and a single nil
// keeps resolveVersion's signature to what a test needs to construct.
func buildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}

	return info
}
