package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// acceptedValue is a spelling Set must take and what the flag prints afterwards.
type acceptedValue struct{ set, want string }

// TestFlagValues covers both pflag.Value adapters as pflag sees them: the default
// through the caller's pointer, the help text, and every accepted spelling.
func TestFlagValues(t *testing.T) {
	var (
		workers int
		timeout time.Duration
	)

	tests := []struct {
		name          string
		register      func(*cobra.Command)
		get           func() string
		wantType      string
		wantDef       string
		accepted      []acceptedValue
		badValue      string
		wantFragments []string
		// noStrconv is #50 itself: strconv's internals reaching the operator.
		noStrconv bool
	}{
		{
			name: "workers",
			register: func(c *cobra.Command) {
				c.Flags().VarP(newWorkersValue(8, &workers), "workers", "w", "number of parallel workers")
			},
			get:           func() string { return strconv.Itoa(workers) },
			wantType:      "int",
			wantDef:       "8",
			accepted:      []acceptedValue{{set: "4", want: "4"}},
			badValue:      "abc",
			wantFragments: []string{"workers", "abc", "want a whole number"},
			noStrconv:     true,
		},
		{
			name: "timeout",
			register: func(c *cobra.Command) {
				c.Flags().VarP(newDurationValue(90*time.Second, &timeout), "timeout", "t", "how long to run")
			},
			get:      func() string { return timeout.String() },
			wantType: "duration",
			wantDef:  "1m30s",
			// The bare-seconds spelling has no workersValue counterpart.
			accepted:      []acceptedValue{{set: "5m", want: "5m0s"}, {set: "300", want: "5m0s"}},
			badValue:      "5 minutes",
			wantFragments: []string{"timeout", "5 minutes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cobra.Command{Use: "test"}
			tt.register(c)

			f := c.Flags().Lookup(tt.name)
			if f == nil {
				t.Fatalf("Lookup(%q) = nil, want the registered flag", tt.name)
			}

			if f.Value.Type() != tt.wantType {
				t.Errorf("%s type = %q, want %q", tt.name, f.Value.Type(), tt.wantType)
			}
			if f.DefValue != tt.wantDef {
				t.Errorf("%s default = %q, want %q", tt.name, f.DefValue, tt.wantDef)
			}
			if got := tt.get(); got != tt.wantDef {
				t.Errorf("%s = %q, want the default written through the pointer", tt.name, got)
			}

			for _, a := range tt.accepted {
				if err := c.Flags().Set(tt.name, a.set); err != nil {
					t.Fatalf("Set(%q) error = %v, want nil", a.set, err)
				}
				if got := tt.get(); got != a.want {
					t.Errorf("%s = %q after Set(%q), want %q", tt.name, got, a.set, a.want)
				}
				if got := f.Value.String(); got != a.want {
					t.Errorf("%s prints as %q after Set(%q), want %q", tt.name, got, a.set, a.want)
				}
			}

			err := c.Flags().Set(tt.name, tt.badValue)
			if err == nil {
				t.Fatalf("Set(%q) error = nil, want a parse error", tt.badValue)
			}

			for _, fragment := range tt.wantFragments {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("Set(%q) error = %q, want it to contain %q", tt.badValue, err, fragment)
				}
			}

			if tt.noStrconv && strings.Contains(err.Error(), "strconv") {
				t.Errorf("Set(%q) error = %q, want no strconv internals in it (#50)", tt.badValue, err)
			}
		})
	}
}
