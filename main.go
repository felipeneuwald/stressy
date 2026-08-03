package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// envPrefix is the prefix on the environment variables that configure stressy:
// --workers is read from STRESSY_WORKERS, --timeout from STRESSY_TIMEOUT.
const envPrefix = "STRESSY"

var (
	version = "0.0.0"
	cmd     = newCmd(&stressy.Cfg{})
)

// newCmd builds the stressy command with its flags registered against cfg. It
// is a constructor rather than a package-level literal so that tests can build
// an independent command over the same definitions.
func newCmd(cfg *stressy.Cfg) *cobra.Command {
	root := &cobra.Command{
		Use:   "stressy",
		Short: "Stressy is a simple tool to perform CPU stress tests",
		Long: `Stressy is a simple tool to perform CPU stress tests.

All flags can be configured using environment variables with the STRESSY_ prefix.
For example: STRESSY_WORKERS=4 or STRESSY_TIMEOUT=5m.`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
		// Every flag not given on the command line is resolved from the
		// environment before the run starts; command-line flags win. See
		// bindEnv.
		PreRunE: func(c *cobra.Command, _ []string) error {
			return bindEnv(c, envPrefix)
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return stressy.New(*cfg).Run()
		},
	}

	root.Flags().IntVarP(&cfg.Workers, "workers", "w", 1, "number of parallel workers for CPU stress testing")

	// Var rather than DurationVarP: the stock pflag duration type would reject
	// the bare-seconds spelling this project has accepted since 0.1.0. See
	// durationValue.
	root.Flags().VarP(newDurationValue(0, &cfg.Timeout), "timeout", "t", "how long to run the stress test, as a duration such as 30s or 5m; a bare number is seconds, and 0 runs until interrupted")

	return root
}

func main() {
	if err := cmd.Execute(); err != nil {
		// cobra has already printed the error.
		os.Exit(1)
	}
}
