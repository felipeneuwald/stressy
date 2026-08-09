package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// bindEnv resolves each setting not given on the command line from the
// environment: under prefix "STRESSY", --workers is read from STRESSY_WORKERS.
// It returns the variable each setting was filled from, keyed by the long flag
// name, which is what lets a later error name it — after this, `-w` and
// STRESSY_WORKERS look alike.
//
// It walks flags, never fs. stressy registers --help and --version itself, and
// nothing on a registered flag says which of them a run can be configured from,
// so the table's own env is the whole of the exclusion: iterating the set
// instead would hand STRESSY_VERSION a value to abort every run with (#47).
func bindEnv(fs *flag.FlagSet, flags []setting, prefix string) (map[string]string, error) {
	given := givenOnCommandLine(fs, flags)

	fromEnv := make(map[string]string)

	for _, s := range flags {
		if !s.env || given[s.long] {
			continue
		}

		name := envName(prefix, s.long)

		// An empty value counts as unset: STRESSY_WORKERS=${WORKERS} with WORKERS
		// undefined is a live compose file and pod spec shape.
		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			continue
		}

		// A Value may write before returning a parse error, so discarding this
		// would turn a bounded run into an endless one.
		if err := s.value.Set(val); err != nil {
			return fromEnv, fmt.Errorf("invalid %v: %w", name, err)
		}

		fromEnv[s.long] = name
	}

	return fromEnv, nil
}

// givenOnCommandLine reports which settings were set, by long name. Visit
// reports the spelling that was typed — `-w 4` marks `w`, not `workers` — so
// each is resolved back through the table first, or STRESSY_WORKERS would
// overwrite a `-w 4` a moment later.
func givenOnCommandLine(fs *flag.FlagSet, flags []setting) map[string]bool {
	long := make(map[string]string, 2*len(flags))

	for _, s := range flags {
		long[s.long] = s.long
		if s.short != "" {
			long[s.short] = s.long
		}
	}

	given := make(map[string]bool, len(flags))

	fs.Visit(func(f *flag.Flag) {
		if name, ok := long[f.Name]; ok {
			given[name] = true
		}
	})

	return given
}

// envName is the prefix, an underscore, and the flag name upper-cased with
// dashes turned into underscores — a dash cannot appear in a portable
// environment variable name. An empty prefix yields the bare name.
func envName(prefix, flagName string) string {
	name := strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
	if prefix == "" {
		return name
	}

	return strings.ToUpper(prefix) + "_" + name
}

// validateRanges rejects a value that parsed but is out of range, naming the
// environment variable it arrived through when it did. This is the last point at
// which that source is known — by the time Run holds the value, `-w 0`,
// STRESSY_WORKERS=0 and the default are one int — and internal/stressy re-applies
// the same rules there, in Cfg.Validate, for callers that do not come through
// this command.
func validateRanges(cfg *stressy.Cfg, fromEnv map[string]string) error {
	setting, err := cfg.Validate()
	if err == nil {
		return nil
	}

	// Deliberately the shape bindEnv gives a value it could not parse.
	if name, ok := fromEnv[setting]; ok {
		return fmt.Errorf("invalid %v: %w", name, err)
	}

	return err
}
