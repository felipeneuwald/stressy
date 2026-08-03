package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/felipeneuwald/stressy/internal/flag"
	"github.com/felipeneuwald/stressy/internal/stressy"
)

var (
	version = "0.0.0"
	c       stressy.Cfg
	cmd     = &cobra.Command{
		Use:   "stressy",
		Short: "Stressy is a simple tool to perform CPU stress tests",
		Long: `Stressy is a simple tool to perform CPU stress tests.

All flags can be configured using environment variables with the STRESSY_ prefix.
For example: STRESSY_WORKERS=4 or STRESSY_TIMEOUT=5m.`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return bindAndValidate(cmd, cobraFlags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s := stressy.New(c)
			return s.Run()
		},
	}
	cobraFlags = newFlags(&c)
)

// newFlags returns the command-line flag definitions, bound to the given
// configuration. It is a constructor rather than a literal so that tests can
// build an independent command over the same definitions.
func newFlags(cfg *stressy.Cfg) []interface{} {
	return []interface{}{
		flag.Int{
			Pointer:          &cfg.Workers,
			FlagName:         "workers",
			FlagShortHand:    "w",
			FlagDefaultValue: 1,
			FlagUsage:        "number of parallel workers for CPU stress testing",
		},
		flag.Duration{
			Pointer:          &cfg.Timeout,
			FlagName:         "timeout",
			FlagShortHand:    "t",
			FlagDefaultValue: 0,
			FlagUsage:        "how long to run the stress test, as a duration such as 30s or 5m; a bare number is seconds, and 0 runs until interrupted",
		},
	}
}

// bindAndValidate resolves each flag that was not set on the command line from
// the environment (STRESSY_ prefix), then validates the resulting values.
// Command-line flags take precedence over environment variables.
func bindAndValidate(cmd *cobra.Command, flags []interface{}) error {
	v := viper.New()
	v.SetEnvPrefix("STRESSY")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if err := flag.Bind(cmd, v); err != nil {
		return err
	}

	return flag.Validate(cmd, flags)
}

func main() {
	if err := flag.Load(cmd, cobraFlags); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading flags: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
