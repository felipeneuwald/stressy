package main

import (
	"os"

	"github.com/felipeneuwald/stressy/internal/cli"
)

// injected is the version stamped into release binaries through `-ldflags "-X
// main.injected=..."`; every other build path leaves it empty. Keep it on one
// `var injected string` line: nothing holds the name to .goreleaser.yaml at
// build time, so TestLdflagsVariableMatchesGoreleaser matches this line as text.
var injected string

func main() { os.Exit(cli.Main(injected)) }
