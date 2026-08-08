package cli

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestParseDuration(t *testing.T) {
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

		// The pre-0.4 spelling. These are the cases that must not regress.
		{name: "bare integer is seconds", in: "300", want: 300 * time.Second},
		{name: "bare zero is indefinite", in: "0", want: 0},
		{name: "bare negative", in: "-1", want: -time.Second},
		{name: "explicitly signed bare integer", in: "+60", want: 60 * time.Second},

		{name: "empty", in: "", wantErr: true},
		{name: "not a number", in: "not-a-number", wantErr: true},
		{name: "unknown unit", in: "5y", wantErr: true},
		{name: "bare float is not seconds", in: "1.5", wantErr: true},
		{name: "overflows time.Duration", in: strconv.FormatInt(math.MaxInt64, 10), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDuration(%q) error = nil, want an error", tt.in)
				}
				// An operator staring at a rejected value needs to see which
				// value was rejected.
				if !strings.Contains(err.Error(), strconv.Quote(tt.in)) {
					t.Errorf("parseDuration(%q) error = %q, want it to name the value", tt.in, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseDuration(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestDurationValue covers durationValue as pflag sees it: the default written
// through the caller's pointer, the placeholder printed in help text, and both
// accepted spellings surviving a Set through a registered flag.
func TestDurationValue(t *testing.T) {
	var timeout time.Duration

	c := &cobra.Command{Use: "test"}
	c.Flags().VarP(newDurationValue(90*time.Second, &timeout), "timeout", "t", "how long to run")

	f := c.Flags().Lookup("timeout")
	if f == nil {
		t.Fatal(`Lookup("timeout") = nil, want the registered flag`)
	}
	// pflag prints Type() as the value placeholder in help output.
	if f.Value.Type() != "duration" {
		t.Errorf("timeout type = %q, want %q", f.Value.Type(), "duration")
	}
	if f.DefValue != "1m30s" {
		t.Errorf("timeout default = %q, want %q", f.DefValue, "1m30s")
	}
	if timeout != 90*time.Second {
		t.Errorf("timeout = %s, want the default written through the pointer", timeout)
	}

	if err := c.Flags().Set("timeout", "5m"); err != nil {
		t.Fatalf("Set() error = %v, want nil", err)
	}
	if timeout != 5*time.Minute {
		t.Errorf("timeout = %s, want %s", timeout, 5*time.Minute)
	}

	if err := c.Flags().Set("timeout", "300"); err != nil {
		t.Fatalf("Set() error = %v, want the bare-seconds form to keep working", err)
	}
	if timeout != 300*time.Second {
		t.Errorf("timeout = %s, want %s", timeout, 300*time.Second)
	}

	// pflag wraps the parse error, so both the flag name and the offending
	// value have to survive to reach the operator.
	err := c.Flags().Set("timeout", "5 minutes")
	if err == nil {
		t.Fatal("Set() error = nil, want a parse error")
	}
	if !strings.Contains(err.Error(), "timeout") || !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("Set() error = %q, want it to name the flag and the bad value", err)
	}
}
