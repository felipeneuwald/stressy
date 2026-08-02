package flag

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Bind connects cobra command flags with viper configuration values.
// It synchronizes flag values with their corresponding configuration values in viper,
// allowing flags to be set via environment variables or config files.
//
// For each flag that hasn't been explicitly set on the command line:
//   - Checks if a corresponding value exists in viper
//   - If found, sets the flag's value to match the viper configuration
//
// The function takes:
//   - cmd: The cobra.Command containing the flags to bind
//   - v: A configured viper instance with loaded configuration values
//
// This enables a priority order where command-line flags take precedence over
// configuration values from viper (environment variables or config files).
//
// Returns an error if either cmd or v is nil, or if a configured value cannot
// be parsed into its flag's type.
func Bind(cmd *cobra.Command, v *viper.Viper) error {
	if cmd == nil || v == nil {
		return fmt.Errorf("cmd or v is nil")
	}

	// VisitAll has no early exit, so record the first failure and skip the
	// remaining flags rather than reporting the last one.
	var err error

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}

		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)

			// pflag writes the zero value before returning a parse error, so
			// discarding this would leave the flag at 0 and look like a
			// deliberate default — turning a bounded run into an endless one.
			if setErr := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); setErr != nil {
				err = fmt.Errorf("invalid configured value for --%v: %w", f.Name, setErr)
			}
		}
	})

	return err
}
