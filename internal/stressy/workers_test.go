package stressy

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestParseWorkers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
		// wantErr is a fragment the message must carry, empty where it parses.
		wantErr string
	}{
		{name: "one", in: "1", want: 1},
		{name: "several", in: "8", want: 8},
		{name: "explicitly signed", in: "+4", want: 4},

		{name: "zero parses; validation rejects it later", in: "0", want: 0},
		{name: "negative parses; validation rejects it later", in: "-1", want: -1},

		{name: "empty", in: "", wantErr: "want a whole number"},
		{name: "not a number", in: "abc", wantErr: "want a whole number"},
		{name: "float", in: "2.0", wantErr: "want a whole number"},
		{name: "trailing space", in: "4 ", wantErr: "want a whole number"},
		{name: "overflows int", in: "99999999999999999999", wantErr: "out of range"},
		{name: "underflows int", in: "-99999999999999999999", wantErr: "out of range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWorkers(tt.in)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("parseWorkers(%q) error = %v, want nil", tt.in, err)
				}
				if got != tt.want {
					t.Errorf("parseWorkers(%q) = %d, want %d", tt.in, got, tt.want)
				}

				return
			}

			if err == nil {
				t.Fatalf("parseWorkers(%q) error = nil, want an error", tt.in)
			}

			// The flag package wraps this in `invalid value %q for flag -%s:`,
			// so naming the value here would put it in the line twice (#123).
			if strings.Contains(err.Error(), strconv.Quote(tt.in)) {
				t.Errorf("parseWorkers(%q) error = %q, want the guidance alone: the flag package names the value (#123)", tt.in, err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseWorkers(%q) error = %q, want it to contain %q", tt.in, err, tt.wantErr)
			}

			if strings.Contains(err.Error(), "strconv") {
				t.Errorf("parseWorkers(%q) error = %q, want no strconv internals in it (#50)", tt.in, err)
			}
		})
	}
}

// TestParseWorkersRangeNamesNoCeiling replaces the test that pinned the number
// the out-of-range message stated through 0.5.0. That number was math.MaxInt:
// the platform's, so it was true, and nine quintillion workers, so it was no
// use to anyone typing a command line (#129). Nothing took its place — a real
// ceiling would have to be measured off the machine, which #104 settled — so
// what is pinned here is that the int width stays out of the message.
func TestParseWorkersRangeNamesNoCeiling(t *testing.T) {
	_, err := parseWorkers("99999999999999999999")
	if err == nil {
		t.Fatal("parseWorkers() error = nil, want the value to be rejected")
	}

	if strings.Contains(err.Error(), strconv.Itoa(math.MaxInt)) {
		t.Errorf("parseWorkers() error = %q, want %d left out of it: an int width is not guidance (#129)", err, math.MaxInt)
	}
}
