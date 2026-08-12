package stressy

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDurationValueSet covers the spellings `--timeout` and `--report` take.
//
// The bare count of seconds is the point of the rejected half: it was accepted
// from 0.1.0 to 0.5.0, so `-t 60` has to fail loudly rather than mean a minute
// to one release and an hour of head-scratching to the next.
func TestDurationValueSet(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "seconds", in: "30s", want: 30 * time.Second},
		{name: "minutes", in: "5m", want: 5 * time.Minute},
		{name: "hours", in: "2h", want: 2 * time.Hour},
		{name: "compound", in: "1h30m", want: 90 * time.Minute},
		{name: "sub-second", in: "250ms", want: 250 * time.Millisecond},
		{name: "fractional", in: "1.5s", want: 1500 * time.Millisecond},
		{name: "negative duration parses; validation rejects it later", in: "-5s", want: -5 * time.Second},
		// Go's own parser takes a bare "0" and nothing else bare, so `-t 0`
		// still means indefinite where `-t 60` no longer means a minute.
		{name: "bare zero is indefinite", in: "0", want: 0},

		{name: "bare integer is no longer seconds", in: "300", wantErr: true},
		{name: "explicitly signed bare integer", in: "+60", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "not a number", in: "not-a-number", wantErr: true},
		{name: "unknown unit", in: "5y", wantErr: true},
		{name: "bare float", in: "1.5", wantErr: true},
		// time.ParseDuration reports its own overflow, so the guard the
		// bare-seconds multiplication needed went with it.
		{name: "overflows time.Duration", in: "10000000h", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got time.Duration

			err := newDurationValue(0, &got).Set(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Set(%q) error = nil, want an error", tt.in)
				}
				// The flag package wraps this in `invalid value %q for flag
				// -%s:`, so naming the value here would print it twice (#123).
				if strings.Contains(err.Error(), strconv.Quote(tt.in)) {
					t.Errorf("Set(%q) error = %q, want the guidance alone: the flag package names the value (#123)", tt.in, err)
				}
				if !strings.Contains(err.Error(), "want a duration such as 30s or 5m") {
					t.Errorf("Set(%q) error = %q, want it to say what a valid value looks like", tt.in, err)
				}
				// The value must be left as it was: a Set that wrote first would
				// turn a rejected command line into a run of some other length.
				if got != 0 {
					t.Errorf("Set(%q) wrote %s through the pointer, want a rejected value to write nothing", tt.in, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("Set(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Set(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
