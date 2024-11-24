package flag

import (
	"reflect"
	"testing"
)

func TestSliceStringToStringReadable(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name:  "empty slice",
			input: []string{},
			want:  "",
		},
		{
			name:  "single element",
			input: []string{"foo"},
			want:  `"foo"`,
		},
		{
			name:  "multiple elements",
			input: []string{"foo", "bar", "baz"},
			want:  `"foo", "bar", "baz"`,
		},
		{
			name:  "elements with spaces",
			input: []string{"hello world", "foo bar"},
			want:  `"hello world", "foo bar"`,
		},
		{
			name:  "elements with special characters",
			input: []string{"!@#$", "%^&*"},
			want:  `"!@#$", "%^&*"`,
		},
		{
			name:  "elements with quotes",
			input: []string{`"quoted"`, `'single'`},
			want:  `""quoted"", "'single'"`,
		},
		{
			name:  "elements with unicode",
			input: []string{"你好", "世界"},
			want:  `"你好", "世界"`,
		},
		{
			name:  "elements with empty strings",
			input: []string{"", "foo", ""},
			want:  `"", "foo", ""`,
		},
		{
			name:  "nil slice",
			input: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceStringToStringReadable(tt.input)
			if got != tt.want {
				t.Errorf("sliceStringToStringReadable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSliceStringToStringReadable_Consistency(t *testing.T) {
	// Test that multiple calls with the same input produce the same output
	input := []string{"foo", "bar"}
	first := sliceStringToStringReadable(input)
	second := sliceStringToStringReadable(input)

	if first != second {
		t.Errorf("Inconsistent results for same input: first = %v, second = %v", first, second)
	}
}

func TestSliceStringToStringReadable_ModifyInput(t *testing.T) {
	// Test that modifying the input slice after calling the function doesn't affect the result
	input := []string{"foo", "bar"}
	result := sliceStringToStringReadable(input)
	expected := `"foo", "bar"`

	// Modify the input slice
	input[0] = "modified"
	input[1] = "changed"

	if result != expected {
		t.Errorf("Result was affected by modifying input slice: got %v, want %v", result, expected)
	}
}

func TestSliceIntToSliceString(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []string
	}{
		{
			name:  "empty slice",
			input: []int{},
			want:  []string{},
		},
		{
			name:  "single element",
			input: []int{42},
			want:  []string{"42"},
		},
		{
			name:  "multiple elements",
			input: []int{1, 2, 3},
			want:  []string{"1", "2", "3"},
		},
		{
			name:  "negative numbers",
			input: []int{-1, -42, -999},
			want:  []string{"-1", "-42", "-999"},
		},
		{
			name:  "zero values",
			input: []int{0, 0, 0},
			want:  []string{"0", "0", "0"},
		},
		{
			name:  "mixed positive and negative",
			input: []int{-1, 0, 1},
			want:  []string{"-1", "0", "1"},
		},
		{
			name:  "large numbers",
			input: []int{999999999, -999999999},
			want:  []string{"999999999", "-999999999"},
		},
		{
			name:  "nil slice",
			input: nil,
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceIntToSliceString(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sliceIntToSliceString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSliceIntToSliceString_Consistency(t *testing.T) {
	// Test that multiple calls with the same input produce the same output
	input := []int{1, 2, 3}
	first := sliceIntToSliceString(input)
	second := sliceIntToSliceString(input)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("Inconsistent results for same input: first = %v, second = %v", first, second)
	}
}

func TestSliceIntToSliceString_ModifyInput(t *testing.T) {
	// Test that modifying the input slice after calling the function doesn't affect the result
	input := []int{1, 2, 3}
	result := sliceIntToSliceString(input)
	expected := []string{"1", "2", "3"}

	// Modify the input slice
	input[0] = 42
	input[1] = 99
	input[2] = -1

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result was affected by modifying input slice: got %v, want %v", result, expected)
	}
}

func TestSliceIntToSliceString_ResultModification(t *testing.T) {
	// Test that modifying the result doesn't affect subsequent calls
	input := []int{1, 2, 3}
	result := sliceIntToSliceString(input)
	
	// Modify the result
	result[0] = "modified"
	result[1] = "changed"
	result[2] = "altered"

	// Get a new result
	newResult := sliceIntToSliceString(input)
	expected := []string{"1", "2", "3"}

	if !reflect.DeepEqual(newResult, expected) {
		t.Errorf("New result was affected by modifying previous result: got %v, want %v", newResult, expected)
	}
}
