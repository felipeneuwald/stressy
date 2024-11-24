package flag

import (
	"fmt"

	"github.com/felipeneuwald/stressy/internal/ptr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagValidate represents a flag validation configuration.
// It contains the flag's name, its current value, and a list of allowed values
// that the flag can take. This is used to validate command-line flags against
// a predefined set of acceptable values.
type flagValidate struct {
	name          string   // name of the flag being validated
	value         string   // current value of the flag
	allowedValues []string // list of valid values that the flag can accept
}

// Validate checks if all flag values in the cobra.Command are valid according to their constraints.
// It performs the following validations:
//   - Ensures all flags are of supported types (String or Int)
//   - For flags with AllowedValues set, verifies the current value is in that list
//   - Skips validation for flags that don't have AllowedValues defined
//
// The function takes:
//   - cmd: The cobra.Command containing the flags to validate
//   - flags: A slice of flag definitions (String or Int types)
//
// Returns an error if:
//   - An unsupported flag type is found
//   - A flag's value is not in its AllowedValues list
func Validate(cmd *cobra.Command, flags []interface{}) error {
	var flagsValidate []flagValidate
	var unsupportedFlagType *string

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		for _, flag := range flags {
			switch v := flag.(type) {
			case String:
				if f.Name != v.FlagName {
					continue
				}

				flagsValidate = append(flagsValidate, flagValidate{
					name:          f.Name,
					value:         f.Value.String(),
					allowedValues: v.AllowedValues,
				})

			case Int:
				if f.Name != v.FlagName {
					continue
				}

				flagsValidate = append(flagsValidate, flagValidate{
					name:          f.Name,
					value:         f.Value.String(),
					allowedValues: sliceIntToSliceString(v.AllowedValues),
				})

			default:
				unsupportedFlagType = ptr.StrPtr(fmt.Sprintf("%T", v))
			}
		}
	})

	if unsupportedFlagType != nil {
		return fmt.Errorf("can't validate unsupported flag type: %v", *unsupportedFlagType)
	}

	for _, v := range flagsValidate {
		if len(v.allowedValues) == 0 {
			continue
		}

		var allowedValue bool

		for _, vv := range v.allowedValues {
			if v.value == vv {
				allowedValue = true
			}
		}

		if !allowedValue {
			return fmt.Errorf(`invalid value "%v" for --%v; allowed values: %v`, v.value, v.name, sliceStringToStringReadable(v.allowedValues))
		}
	}

	return nil
}
