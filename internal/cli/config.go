package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// bindEnv resolves each flag not given on the command line from the environment:
// under prefix "STRESSY", --workers is read from STRESSY_WORKERS. A flag pflag
// records as Changed is left alone, and cobra's own flags are skipped. It returns
// the variable each flag was filled from, keyed by flag name, which is what lets
// a later error name it — after this, `-w` and STRESSY_WORKERS look alike.
func bindEnv(cmd *cobra.Command, prefix string) (map[string]string, error) {
	// VisitAll has no early exit, so record the first failure and skip the
	// remaining flags rather than reporting the last one.
	var err error

	fromEnv := make(map[string]string)

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil || f.Changed {
			return
		}

		// cobra acts on --help and --version while parsing flags, before this
		// runs, so an environment value for either could only ever fail the run.
		if SetByCobra(f) {
			return
		}

		name := envName(prefix, f.Name)

		// An empty value counts as unset: STRESSY_WORKERS=${WORKERS} with WORKERS
		// undefined is a live compose file and pod spec shape.
		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			return
		}

		// pflag writes the zero value before returning a parse error, so
		// discarding this would turn a bounded run into an endless one.
		if setErr := cmd.Flags().Set(f.Name, val); setErr != nil {
			err = fmt.Errorf("invalid %v: %w", name, setErr)

			return
		}

		fromEnv[f.Name] = name
	})

	return fromEnv, err
}

// SetByCobra reports whether cobra registered this flag itself rather than
// NewCmd declaring it, by reading cobra's own annotation rather than a list of
// names kept here. TestCobraAnnotatesItsOwnFlags pins that annotation against
// the cobra in go.mod, because a bump dropping it would be silent.
func SetByCobra(f *pflag.Flag) bool {
	return len(f.Annotations[cobra.FlagSetByCobraAnnotation]) > 0
}

// EnvName is the variable a flag of this name is filled from, with stressy's
// own prefix baked in — which is what a caller outside this package wants.
func EnvName(flagName string) string {
	return envName(envPrefix, flagName)
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
