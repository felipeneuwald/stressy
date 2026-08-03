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
// pass.
//
// Returns an error if an environment value cannot be parsed into its flag's
// type.
func bindEnv(cmd *cobra.Command, prefix string) error {
	// VisitAll has no early exit, so record the first failure and skip the
	// remaining flags rather than reporting the last one.
	var err error

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil || f.Changed {
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
		}
	})

	return err
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
