package main

import (
	"os"
	"regexp"
	"runtime/debug"
	"testing"
)

// TestResolveVersion walks every build path stressy has, which is the point of
// #40: five of the six reported "0.0.0" before, including `go install`, the
// installation method the README lists first.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name     string
		injected string
		// mainVersion is debug.BuildInfo.Main.Version. The empty string with
		// noBuildInfo unset is the "recorded, but blank" case; noBuildInfo is
		// ReadBuildInfo failing outright.
		mainVersion string
		noBuildInfo bool
		want        string
	}{
		{
			name:        "goreleaser release",
			injected:    "0.4.0",
			mainVersion: "v0.4.0",
			want:        "0.4.0",
		},
		{
			// The stamped value wins even where build info disagrees: it is
			// the version the release was cut as.
			name:        "injected value beats build info",
			injected:    "0.4.0",
			mainVersion: "v0.3.3",
			want:        "0.4.0",
		},
		{
			name:        "go install at a tag",
			mainVersion: "v0.4.0",
			want:        "0.4.0",
		},
		{
			name:        "go build on a dirty tree",
			mainVersion: "v0.4.0+dirty",
			want:        "0.4.0+dirty",
		},
		{
			// HEAD past the last tag: the toolchain records a pseudo-version,
			// which identifies the exact commit and is kept whole.
			name:        "go build past the last tag",
			mainVersion: "v0.4.1-0.20260803201049-f12897df761f",
			want:        "0.4.1-0.20260803201049-f12897df761f",
		},
		{
			name:        "go build -buildvcs=false",
			mainVersion: "(devel)",
			want:        devVersion,
		},
		{
			name:        "build info recorded no version",
			mainVersion: "",
			want:        devVersion,
		},
		{
			name:        "no build info at all",
			noBuildInfo: true,
			want:        devVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info *debug.BuildInfo
			if !tt.noBuildInfo {
				info = &debug.BuildInfo{Main: debug.Module{Version: tt.mainVersion}}
			}

			if got := resolveVersion(tt.injected, info); got != tt.want {
				t.Errorf("resolveVersion(%q, %q) = %q, want %q", tt.injected, tt.mainVersion, got, tt.want)
			}
		})
	}
}

// TestVersionIsReported guards the whole of what #40 is about at the level the
// user sees it: whatever this binary was built by, `--version` has to print
// something that identifies it. The test binary itself is the awkward case —
// `go test` records no module version for it — so this asserts the property
// that holds on every path rather than a value.
func TestVersionIsReported(t *testing.T) {
	if version == "" {
		t.Fatal("version = \"\", want a version or the development placeholder")
	}

	if version == "0.0.0" {
		t.Errorf("version = %q, the placeholder every non-release build used to report (#40)", version)
	}
}

// ldflagsVar matches the variable .goreleaser.yaml stamps, capturing the name
// out of `-s -w -X main.injected={{.Version}}`.
var ldflagsVar = regexp.MustCompile(`-X main\.(\w+)=`)

// TestLdflagsVariableMatchesGoreleaser pins the one failure in this area that
// cannot announce itself. `-X main.whatever=` naming a variable that does not
// exist is not a link error — the value is dropped, and the binary quietly
// reports the build-info fallback instead of the version it was released as.
// Renaming the variable on either side, or deleting it, therefore has to fail
// here or it fails in a published release.
func TestLdflagsVariableMatchesGoreleaser(t *testing.T) {
	const config = ".goreleaser.yaml"

	cfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the release config", config, err)
	}

	match := ldflagsVar.FindSubmatch(cfg)
	if match == nil {
		t.Fatalf("%s has no `-X main.<var>=` in its ldflags; releases would report the build-info fallback", config)
	}

	// Matched against the source rather than against a literal here: a rename
	// that updates only one of the two files has to be what fails.
	name := string(match[1])
	decl := regexp.MustCompile(`(?m)^var ` + regexp.QuoteMeta(name) + ` string$`)

	const source = "version.go"

	src, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the version source", source, err)
	}

	if !decl.Match(src) {
		t.Errorf("%s stamps main.%s, which %s does not declare as `var %s string`", config, name, source, name)
	}
}
