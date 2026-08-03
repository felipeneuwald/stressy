package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// testPrefix rather than STRESSY, so that a STRESSY_* variable exported in the
// shell running `go test` cannot reach these cases. The "nothing set" rows only
// mean anything if the variable is genuinely absent.
const testPrefix = "FLAGTEST"

// newCmdWithIntFlag builds a bare command carrying a single int flag, so that
// these cases exercise bindEnv rather than stressy's own flag set.
func newCmdWithIntFlag(t *testing.T, name string, p *int, defaultValue int) *cobra.Command {
	t.Helper()

	c := &cobra.Command{Use: "test"}
	c.Flags().IntVar(p, name, defaultValue, "number of workers")

	return c
}

func TestBindEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		commandLine string
		want        int
	}{
		{name: "nothing set keeps the default", want: 1},
		{name: "environment value is applied", env: map[string]string{"FLAGTEST_WORKERS": "8"}, want: 8},
		{name: "command line wins over the environment", env: map[string]string{"FLAGTEST_WORKERS": "8"}, commandLine: "2", want: 2},
		{name: "command line is kept when the environment is unset", commandLine: "2", want: 2},
		// An empty value is what STRESSY_WORKERS=${WORKERS} expands to when
		// WORKERS is undefined. It has always been ignored; see bindEnv.
		{name: "empty environment value is treated as unset", env: map[string]string{"FLAGTEST_WORKERS": ""}, want: 1},
		// The prefix is what keeps a flag from picking up an unrelated
		// variable that happens to share its name.
		{name: "unprefixed variable is not read", env: map[string]string{"WORKERS": "8"}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var workers int
			c := newCmdWithIntFlag(t, "workers", &workers, 1)

			// Setting through Flags() marks the flag as Changed, which is how
			// pflag records "the user passed this on the command line".
			if tt.commandLine != "" {
				if err := c.Flags().Set("workers", tt.commandLine); err != nil {
					t.Fatalf("Set() error = %v, want nil", err)
				}
			}

			if err := bindEnv(c, testPrefix); err != nil {
				t.Fatalf("bindEnv() error = %v, want nil", err)
			}

			if workers != tt.want {
				t.Errorf("workers = %d, want %d", workers, tt.want)
			}
		})
	}
}

// TestBindEnvMalformedValue covers the half of #10 that lives here: a value
// pflag cannot parse must surface as an error rather than leaving the flag at
// its zero value, and the message has to name the variable that caused it —
// that name is the only thing the operator can act on.
func TestBindEnvMalformedValue(t *testing.T) {
	t.Setenv("FLAGTEST_WORKERS", "not-a-number")

	var workers int
	c := newCmdWithIntFlag(t, "workers", &workers, 1)

	err := bindEnv(c, testPrefix)
	if err == nil {
		t.Fatal("bindEnv() error = nil, want the malformed value to be rejected")
	}

	if !strings.Contains(err.Error(), "FLAGTEST_WORKERS") || !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("bindEnv() error = %q, want it to name the variable and the bad value", err)
	}
}

// TestBindEnvMultiWordFlagName is the case envName's dash substitution exists
// for: a dash is not portable in an environment variable name, so --max-retries
// has to be reachable as FLAGTEST_MAX_RETRIES or not at all.
func TestBindEnvMultiWordFlagName(t *testing.T) {
	t.Setenv("FLAGTEST_MAX_RETRIES", "3")

	var retries int
	c := newCmdWithIntFlag(t, "max-retries", &retries, 0)

	if err := bindEnv(c, testPrefix); err != nil {
		t.Fatalf("bindEnv() error = %v, want nil", err)
	}

	if retries != 3 {
		t.Errorf("max-retries = %d, want 3", retries)
	}
}

func TestEnvName(t *testing.T) {
	tests := []struct {
		prefix   string
		flagName string
		want     string
	}{
		{prefix: "STRESSY", flagName: "workers", want: "STRESSY_WORKERS"},
		{prefix: "STRESSY", flagName: "max-retries", want: "STRESSY_MAX_RETRIES"},
		{prefix: "stressy", flagName: "workers", want: "STRESSY_WORKERS"},
		{prefix: "", flagName: "workers", want: "WORKERS"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := envName(tt.prefix, tt.flagName); got != tt.want {
				t.Errorf("envName(%q, %q) = %q, want %q", tt.prefix, tt.flagName, got, tt.want)
			}
		})
	}
}
