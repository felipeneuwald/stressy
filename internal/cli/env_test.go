package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// testPrefix rather than STRESSY, so an ambient STRESSY_* cannot reach a case.
const testPrefix = "FLAGTEST"

// newCmdWithIntFlag builds a bare command carrying a single int flag.
func newCmdWithIntFlag(t *testing.T, name string, p *int, defaultValue int) *cobra.Command {
	t.Helper()

	c := &cobra.Command{Use: "test"}
	c.Flags().IntVar(p, name, defaultValue, "number of workers")

	return c
}

// newCmdWithCobraFlags adds cobra's own --help and --version the way Execute does.
func newCmdWithCobraFlags(t *testing.T, p *int) *cobra.Command {
	t.Helper()

	// InitDefaultVersionFlag registers nothing unless Version is set.
	c := newCmdWithIntFlag(t, "workers", p, 1)
	c.Version = "0.0.0-test"
	c.InitDefaultHelpFlag()
	c.InitDefaultVersionFlag()

	return c
}

func TestBindEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		commandLine string
		want        int
		// wantSource is what pflag's own Changed cannot answer (#51).
		wantSource string
	}{
		{name: "nothing set keeps the default", want: 1},
		{name: "environment value is applied", env: map[string]string{"FLAGTEST_WORKERS": "8"}, want: 8, wantSource: "FLAGTEST_WORKERS"},
		{name: "command line wins over the environment", env: map[string]string{"FLAGTEST_WORKERS": "8"}, commandLine: "2", want: 2},
		{name: "command line is kept when the environment is unset", commandLine: "2", want: 2},
		{name: "empty environment value is treated as unset", env: map[string]string{"FLAGTEST_WORKERS": ""}, want: 1},
		{name: "unprefixed variable is not read", env: map[string]string{"WORKERS": "8"}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var workers int
			c := newCmdWithIntFlag(t, "workers", &workers, 1)

			if tt.commandLine != "" {
				if err := c.Flags().Set("workers", tt.commandLine); err != nil {
					t.Fatalf("Set() error = %v, want nil", err)
				}
			}

			fromEnv, err := bindEnv(c, testPrefix)
			if err != nil {
				t.Fatalf("bindEnv() error = %v, want nil", err)
			}

			if workers != tt.want {
				t.Errorf("workers = %d, want %d", workers, tt.want)
			}

			if got := fromEnv["workers"]; got != tt.wantSource {
				t.Errorf("bindEnv() filled workers from %q, want %q", got, tt.wantSource)
			}
		})
	}
}

// TestBindEnvMalformedValue: an unparseable value must error, not zero the flag.
func TestBindEnvMalformedValue(t *testing.T) {
	t.Setenv("FLAGTEST_WORKERS", "not-a-number")

	var workers int
	c := newCmdWithIntFlag(t, "workers", &workers, 1)

	fromEnv, err := bindEnv(c, testPrefix)
	if err == nil {
		t.Fatal("bindEnv() error = nil, want the malformed value to be rejected")
	}

	if !strings.Contains(err.Error(), "FLAGTEST_WORKERS") || !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("bindEnv() error = %q, want it to name the variable and the bad value", err)
	}

	// Reporting it as filled would misattribute a later failure.
	if _, ok := fromEnv["workers"]; ok {
		t.Errorf("bindEnv() reports workers as filled from %q, want a rejected value to be reported as no source", fromEnv["workers"])
	}
}

// TestBindEnvIgnoresFlagsCobraSetItself covers #47: cobra acts before bindEnv runs.
func TestBindEnvIgnoresFlagsCobraSetItself(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "values no bool flag can parse", env: map[string]string{"FLAGTEST_HELP": "yes", "FLAGTEST_VERSION": "0.4.0", "FLAGTEST_WORKERS": "8"}},
		{name: "values a bool flag can parse", env: map[string]string{"FLAGTEST_HELP": "true", "FLAGTEST_VERSION": "true", "FLAGTEST_WORKERS": "8"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var workers int
			c := newCmdWithCobraFlags(t, &workers)

			fromEnv, err := bindEnv(c, testPrefix)
			if err != nil {
				t.Fatalf("bindEnv() error = %v, want nil", err)
			}

			for _, name := range []string{"help", "version"} {
				if source, ok := fromEnv[name]; ok {
					t.Errorf("bindEnv() reports --%s as filled from %q, want cobra's own flags left out entirely", name, source)
				}

				f := c.Flags().Lookup(name)
				if f == nil {
					t.Fatalf("Lookup(%q) = nil, want the flag cobra registers", name)
				}

				if f.Changed || f.Value.String() != "false" {
					t.Errorf("--%s = %q, changed = %t; want %q and untouched", name, f.Value.String(), f.Changed, "false")
				}
			}

			if workers != 8 {
				t.Errorf("workers = %d, want 8 from the environment", workers)
			}
		})
	}
}

// TestCobraAnnotatesItsOwnFlags: a cobra bump dropping it would revive #47.
func TestCobraAnnotatesItsOwnFlags(t *testing.T) {
	var workers int
	c := newCmdWithCobraFlags(t, &workers)

	for _, name := range []string{"help", "version"} {
		f := c.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("Lookup(%q) = nil, want cobra to have registered its flag", name)
		}

		if !SetByCobra(f) {
			t.Errorf("--%s carries no %s annotation, which is how bindEnv knows to leave it alone (#47)", name, cobra.FlagSetByCobraAnnotation)
		}
	}

	// And stressy's own flag must not carry it, or the exclusion swallows it.
	f := c.Flags().Lookup("workers")
	if f == nil {
		t.Fatal(`Lookup("workers") = nil, want the registered flag`)
	}

	if SetByCobra(f) {
		t.Errorf("--workers carries the %s annotation, so the exclusion would swallow stressy's own flags", cobra.FlagSetByCobraAnnotation)
	}
}

// TestBindEnvMultiWordFlagName is what envName's dash substitution exists for.
func TestBindEnvMultiWordFlagName(t *testing.T) {
	t.Setenv("FLAGTEST_MAX_RETRIES", "3")

	var retries int
	c := newCmdWithIntFlag(t, "max-retries", &retries, 0)

	fromEnv, err := bindEnv(c, testPrefix)
	if err != nil {
		t.Fatalf("bindEnv() error = %v, want nil", err)
	}

	if retries != 3 {
		t.Errorf("max-retries = %d, want 3", retries)
	}

	// Keyed by flag name: validateRanges looks the source up by the setting.
	if got := fromEnv["max-retries"]; got != "FLAGTEST_MAX_RETRIES" {
		t.Errorf("bindEnv() filled max-retries from %q, want %q", got, "FLAGTEST_MAX_RETRIES")
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
