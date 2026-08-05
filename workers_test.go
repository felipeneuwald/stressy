package main

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseWorkers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
		// wantErr is a fragment the message must carry, empty where the value
		// parses. What it is checking is that the message says what a valid
		// value looks like, which is the whole of #50 — the old one said
		// `strconv.ParseInt: parsing "abc": invalid syntax`.
		wantErr string
	}{
		{name: "one", in: "1", want: 1},
		{name: "several", in: "8", want: 8},
		{name: "explicitly signed", in: "+4", want: 4},

		// Out of range, not unparseable: validateRanges rejects these a moment
		// later, with usage silenced. See parseWorkers.
		{name: "zero parses; validation rejects it later", in: "0", want: 0},
		{name: "negative parses; validation rejects it later", in: "-1", want: -1},

		{name: "empty", in: "", wantErr: "want a whole number"},
		{name: "not a number", in: "abc", wantErr: "want a whole number"},
		// The spelling an operator who has been reading YAML arrives with, and
		// the one #50 quotes.
		{name: "float", in: "2.0", wantErr: "want a whole number"},
		{name: "trailing space", in: "4 ", wantErr: "want a whole number"},
		// strconv reports these as out of range rather than as syntax errors,
		// and so does the message: twenty digits are a whole number, just not
		// one an int can hold.
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

			// An operator staring at a rejected value needs to see which value
			// was rejected, the same way parseDuration names it.
			if !strings.Contains(err.Error(), strconv.Quote(tt.in)) {
				t.Errorf("parseWorkers(%q) error = %q, want it to name the value", tt.in, err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseWorkers(%q) error = %q, want it to contain %q", tt.in, err, tt.wantErr)
			}

			// The bug itself: pflag's stock int flag surfaced strconv's own
			// error, internals included, to a reader who may not write Go.
			if strings.Contains(err.Error(), "strconv") {
				t.Errorf("parseWorkers(%q) error = %q, want no strconv internals in it (#50)", tt.in, err)
			}
		})
	}
}

// TestParseWorkersRangeNamesTheLimit pins the one number the out-of-range
// message states. It is the platform's, not a literal, so a 32-bit build says
// what is true there.
func TestParseWorkersRangeNamesTheLimit(t *testing.T) {
	_, err := parseWorkers("99999999999999999999")
	if err == nil {
		t.Fatal("parseWorkers() error = nil, want the value to be rejected")
	}

	if !strings.Contains(err.Error(), strconv.Itoa(math.MaxInt)) {
		t.Errorf("parseWorkers() error = %q, want it to name the maximum %d", err, math.MaxInt)
	}
}

// TestWorkersValue covers workersValue as pflag sees it: the default written
// through the caller's pointer, the placeholder and default printed in help
// text, and what a rejected value reads like once pflag has wrapped it.
func TestWorkersValue(t *testing.T) {
	var workers int

	c := &cobra.Command{Use: "test"}
	c.Flags().VarP(newWorkersValue(8, &workers), "workers", "w", "number of parallel workers")

	f := c.Flags().Lookup("workers")
	if f == nil {
		t.Fatal(`Lookup("workers") = nil, want the registered flag`)
	}

	// Both of these are what `--help` prints, and #50 asks for neither to
	// change: the placeholder in `-w, --workers int` and the `(default 8)`
	// after the usage text.
	if f.Value.Type() != "int" {
		t.Errorf("workers type = %q, want %q", f.Value.Type(), "int")
	}
	if f.DefValue != "8" {
		t.Errorf("workers default = %q, want %q", f.DefValue, "8")
	}
	if workers != 8 {
		t.Errorf("workers = %d, want the default written through the pointer", workers)
	}

	if err := c.Flags().Set("workers", "4"); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}
	if workers != 4 {
		t.Errorf("workers = %d, want 4", workers)
	}
	if f.Value.String() != "4" {
		t.Errorf("workers prints as %q, want %q", f.Value.String(), "4")
	}

	// pflag wraps the parse error, so the flag name, the offending value and
	// the hint all have to survive to reach the operator.
	err := c.Flags().Set("workers", "abc")
	if err == nil {
		t.Fatal("Set() error = nil, want a parse error")
	}

	for _, want := range []string{"workers", "abc", "want a whole number"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Set() error = %q, want it to contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "strconv") {
		t.Errorf("Set() error = %q, want no strconv internals in it (#50)", err)
	}
}
