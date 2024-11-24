package flag

import (
	"fmt"
	"strconv"
)

// sliceStringToStringReadable converts a slice of strings into a human-readable string
// where each element is quoted and separated by commas.
//
// For example:
//
//	input: []string{"foo", "bar", "baz"}
//	output: "foo", "bar", "baz"
//
// This is used primarily for formatting allowed values in flag usage and error messages.
func sliceStringToStringReadable(xs []string) string {
	var s string

	for i, v := range xs {
		s = s + fmt.Sprintf(`"%v"`, v)

		if i != len(xs)-1 {
			s = s + ", "
		}
	}

	return s
}

// sliceIntToSliceString converts a slice of integers into a slice of strings.
// Each integer is converted to its string representation using strconv.Itoa.
//
// For example:
//
//	input: []int{1, 2, 3}
//	output: []string{"1", "2", "3"}
//
// This is used internally to convert integer flag allowed values to strings
// for consistent validation and error message formatting.
func sliceIntToSliceString(xi []int) []string {
	xs := make([]string, len(xi))

	for i, v := range xi {
		xs[i] = strconv.Itoa(v)
	}

	return xs
}
