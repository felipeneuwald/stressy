package flag

import (
	"reflect"
	"testing"
)

func TestSliceStringToStringReadable(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "empty", in: []string{}, want: ""},
		{name: "single", in: []string{"foo"}, want: `"foo"`},
		{name: "multiple", in: []string{"foo", "bar", "baz"}, want: `"foo", "bar", "baz"`},
		{name: "empty element", in: []string{"", "bar"}, want: `"", "bar"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sliceStringToStringReadable(tt.in); got != tt.want {
				t.Errorf("sliceStringToStringReadable(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSliceIntToSliceString(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []string
	}{
		{name: "nil", in: nil, want: []string{}},
		{name: "empty", in: []int{}, want: []string{}},
		{name: "single", in: []int{1}, want: []string{"1"}},
		{name: "multiple", in: []int{1, 2, 3}, want: []string{"1", "2", "3"}},
		{name: "zero and negative", in: []int{0, -7}, want: []string{"0", "-7"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sliceIntToSliceString(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sliceIntToSliceString(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
