package flag

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Load registers flags with a cobra.Command based on the provided flag definitions.
// For each flag, it:
//   - Registers it with the command using the appropriate type (string or int)
//   - Adds allowed values to the usage text if specified
//   - Handles both long (--flag) and short (-f) flag formats
//
// The function takes:
//   - cmd: The cobra.Command to register flags with
//   - flags: A slice of flag definitions (String or Int types)
//
// For flags with allowed values, the usage text is automatically extended with
// the list of allowed values in parentheses.
//
// Example usage text for a flag with allowed values:
//   --format string   Output format (allowed: "json", "yaml", "text")
//
// Returns an error if an unsupported flag type is provided.
func Load(cmd *cobra.Command, flags []interface{}) error {
	for _, flag := range flags {
		switch v := flag.(type) {
		case String:
			if len(v.AllowedValues) != 0 {
				v.FlagUsage = v.FlagUsage + fmt.Sprintf(" (allowed: %v)", sliceStringToStringReadable(v.AllowedValues))
			}

			if v.FlagShortHand != "" {
				cmd.Flags().StringVarP(v.Pointer, v.FlagName, v.FlagShortHand, v.FlagDefaultValue, v.FlagUsage)
				continue
			}

			cmd.Flags().StringVar(v.Pointer, v.FlagName, v.FlagDefaultValue, v.FlagUsage)

		case Int:
			if len(v.AllowedValues) != 0 {
				v.FlagUsage = v.FlagUsage + fmt.Sprintf(" (allowed: %v)", sliceStringToStringReadable(sliceIntToSliceString(v.AllowedValues)))
			}

			if v.FlagShortHand != "" {
				cmd.Flags().IntVarP(v.Pointer, v.FlagName, v.FlagShortHand, v.FlagDefaultValue, v.FlagUsage)
				continue
			}

			cmd.Flags().IntVar(v.Pointer, v.FlagName, v.FlagDefaultValue, v.FlagUsage)

		default:
			return fmt.Errorf("can't load unsupported flag type: %T", v)
		}
	}

	return nil
}
