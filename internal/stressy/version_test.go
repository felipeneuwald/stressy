package stressy

import (
	"runtime/debug"
	"testing"
)

// TestResolveVersion walks every build path; five of six reported "0.0.0" (#40).
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name     string
		injected string
		// noBuildInfo is ReadBuildInfo failing outright.
		mainVersion string
		noBuildInfo bool
		want        string
	}{
		{name: "goreleaser release", injected: "0.4.0", mainVersion: "v0.4.0", want: "0.4.0"},
		{name: "injected value beats build info", injected: "0.4.0", mainVersion: "v0.3.3", want: "0.4.0"},
		{name: "go install at a tag", mainVersion: "v0.4.0", want: "0.4.0"},
		{name: "go build on a dirty tree", mainVersion: "v0.4.0+dirty", want: "0.4.0+dirty"},
		{name: "go build past the last tag", mainVersion: "v0.4.1-0.20260803201049-f12897df761f", want: "0.4.1-0.20260803201049-f12897df761f"},
		{name: "go build -buildvcs=false", mainVersion: "(devel)", want: devVersion},
		{name: "build info recorded no version", mainVersion: "", want: devVersion},
		{name: "no build info at all", noBuildInfo: true, want: devVersion},
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

// TestVersionIsReported asserts the property: `go test` records no version, and
// the command built inside one still has something to print. Read off a real
// command rather than off resolveVersion, which is where the build path this
// binary took reaches --version.
func TestVersionIsReported(t *testing.T) {
	version := newCmd(&Cfg{}, "").version

	if version == "" {
		t.Fatal("version = \"\", want a version or the development placeholder")
	}

	if version == "0.0.0" {
		t.Errorf("version = %q, the placeholder every non-release build used to report (#40)", version)
	}
}
