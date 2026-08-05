package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// bindEnv resolves each flag that was not given on the command line from the
// environment, so that a flag can be set either way.
//
// The variable a flag is read from is its name, upper-cased and prefixed:
// under prefix "STRESSY", --workers is STRESSY_WORKERS. See envName for the
// exact mapping.
//
// Command-line flags take precedence: a flag pflag has recorded as Changed is
// left alone, so the environment only ever fills in what the operator did not
// pass. The flags cobra registers for itself are skipped entirely; see
// setByCobra.
//
// Returns the variable each flag was filled from, keyed by flag name, and an
// error if an environment value cannot be parsed into its flag's type. The map
// is what lets a later check name the source of a value it rejects: setting a
// flag from the environment goes through the same Set that records Changed, so
// once this has run a value from STRESSY_WORKERS and one from `-w` are
// indistinguishable (#51). See validateRanges.
func bindEnv(cmd *cobra.Command, prefix string) (map[string]string, error) {
	// VisitAll has no early exit, so record the first failure and skip the
	// remaining flags rather than reporting the last one.
	var err error

	fromEnv := make(map[string]string)

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil || f.Changed {
			return
		}

		// --help and --version are cobra's rather than this command's, and
		// cobra acts on both while parsing flags — before PreRunE, which is
		// where this runs. An environment value for either could therefore
		// never take effect, but it could still fail to parse and take the run
		// down with it: STRESSY_VERSION=0.4.0, a plausible way to pin an image
		// tag in a compose file or a CI environment, aborted every run with a
		// bool-parse error naming a flag the operator never touched (#47).
		if setByCobra(f) {
			return
		}

		name := envName(prefix, f.Name)

		// An empty value counts as unset, which is what this did when the
		// lookup went through viper's AutomaticEnv (AllowEmptyEnv defaults to
		// false). STRESSY_WORKERS=${WORKERS} with WORKERS undefined is a
		// common shape in compose files and pod specs, and it resolves to the
		// empty string; rejecting that would break deployments that run today.
		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			return
		}

		// pflag writes the zero value before returning a parse error, so
		// discarding this would leave the flag at 0 and look like a
		// deliberate default — turning a bounded run into an endless one.
		if setErr := cmd.Flags().Set(f.Name, val); setErr != nil {
			err = fmt.Errorf("invalid %v: %w", name, setErr)

			return
		}

		fromEnv[f.Name] = name
	})

	return fromEnv, err
}

// setByCobra reports whether cobra registered this flag itself rather than
// newCmd declaring it — --help always, and --version because the command sets
// Version. cobra annotates both as its own as it adds them, and reads that same
// annotation to tell its flags from a command's own, so this is the library's
// own distinction rather than a list of names maintained here: a flag the
// command declares under either name would still be configured from the
// environment, and one cobra adds in a future version would not.
//
// TestCobraAnnotatesItsOwnFlags pins the annotation against the cobra in
// go.mod, because if a bump dropped it the only symptom would be #47 returning.
func setByCobra(f *pflag.Flag) bool {
	return len(f.Annotations[cobra.FlagSetByCobraAnnotation]) > 0
}

// envName is the environment variable a flag is read from: the prefix, an
// underscore, and the flag name upper-cased with dashes turned into
// underscores, so --max-retries under prefix "STRESSY" is STRESSY_MAX_RETRIES.
// An empty prefix yields the bare name rather than a leading underscore.
//
// The dash-to-underscore substitution is not decoration: a dash cannot appear
// in a portable environment variable name, so without it every multi-word flag
// would be unreachable from the environment.
func envName(prefix, flagName string) string {
	name := strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
	if prefix == "" {
		return name
	}

	return strings.ToUpper(prefix) + "_" + name
}
