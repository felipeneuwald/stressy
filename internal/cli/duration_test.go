package cli

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
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
