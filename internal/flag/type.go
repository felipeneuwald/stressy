package flag

// String holds a *cobra.Command flag of type string.
// It provides a type-safe way to define string flags with validation support.
type String struct {
	// Pointer points to the variable that will store the flag value
	Pointer *string

	// FlagName is the name of the flag (used with -- prefix)
	FlagName string

	// FlagShortHand is an optional single-character shorthand (used with - prefix)
	FlagShortHand string

	// FlagDefaultValue is used when the flag is not explicitly set
	FlagDefaultValue string

	// FlagUsage is the help text describing the flag's purpose
	FlagUsage string

	// AllowedValues is an optional list of valid values for this flag
	AllowedValues []string
}

// Int holds a *cobra.Command flag of type int.
// It provides a type-safe way to define integer flags with validation support.
type Int struct {
	// Pointer points to the variable that will store the flag value
	Pointer *int

	// FlagName is the name of the flag (used with -- prefix)
	FlagName string

	// FlagShortHand is an optional single-character shorthand (used with - prefix)
	FlagShortHand string

	// FlagUsage is the help text describing the flag's purpose
	FlagUsage string

	// AllowedValues is an optional list of valid values for this flag
	AllowedValues []int

	// FlagDefaultValue is used when the flag is not explicitly set
	FlagDefaultValue int
}
