package cli

import (
	"runtime/debug"
	"strings"
)

// version is what `stressy --version` prints. Resolved here from build info
// alone, so that it is never empty whoever builds the command — a `go test`
// binary included, which is stamped with nothing — and re-resolved by Main with
// the value the root main.go holds.
var version = resolveVersion("", buildInfo())

// devVersion is reported when nothing in the binary knows what it is. "devel"
// says that plainly, where the "0.0.0" it replaces looked like a real release.
const devVersion = "devel"

// resolveVersion reports the version of this binary: the injected value, else
// the module version the toolchain records with its leading "v" stripped, else
// devVersion. The "v" goes because goreleaser's {{ .Version }} has no prefix;
// "+dirty" is kept, because such a binary is not the release it is closest to.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "" {
		return injected
	}

	// "(devel)" is what the toolchain records when it has no version to record.
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return devVersion
	}

	return strings.TrimPrefix(info.Main.Version, "v")
}

// buildInfo is debug.ReadBuildInfo with unavailable and empty folded into nil.
func buildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}

	return info
}
