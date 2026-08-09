package main

import (
	"os"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// injected is the version stamped into release binaries through `-ldflags "-X
// main.injected=..."`; every other build path leaves it empty. Renaming it means
// renaming it in .goreleaser.yaml too: `-X main.whatever=` naming a variable
// that does not exist is not a link error, the value is dropped, and the release
// binary quietly reports the build-info fallback instead (#40).
var injected string

func main() { os.Exit(stressy.Main(injected)) }
