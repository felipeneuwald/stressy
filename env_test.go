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

// newCmdWithCobraFlags builds a command carrying cobra's own --help and
// --version beside a real flag, registered the way Execute registers them. A
// bare cobra.Command has neither, so a case built on one cannot reach #47 at
// all.
func newCmdWithCobraFlags(t *testing.T, p *int) *cobra.Command {
	t.Helper()

	c := newCmdWithIntFlag(t, "workers", p, 1)

	// InitDefaultVersionFlag registers nothing unless Version is set, which is
	// the condition newCmd meets by setting it on the root command.
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

// TestBindEnvIgnoresFlagsCobraSetItself covers #47. cobra registers --help and
// --version itself and acts on both while parsing flags, before PreRunE and so
// before bindEnv — an environment value for either can never take effect, and
// handing one to a bool flag could only ever fail. STRESSY_VERSION=0.4.0, which
// is a plausible way to pin an image tag in a compose file or a CI environment,
// therefore aborted every run with a ParseBool error about a flag the operator
// never touched.
func TestBindEnvIgnoresFlagsCobraSetItself(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		// The half that took the run down.
		{
			name: "values no bool flag can parse",
			env:  map[string]string{"FLAGTEST_HELP": "yes", "FLAGTEST_VERSION": "0.4.0", "FLAGTEST_WORKERS": "8"},
		},
		// And the half that parsed and was then discarded, cobra having already
		// decided not to print help by the time bindEnv ran. Ignoring it
		// deliberately is the same run without the pretence that it configures
		// anything.
		{
			name: "values a bool flag can parse",
			env:  map[string]string{"FLAGTEST_HELP": "true", "FLAGTEST_VERSION": "true", "FLAGTEST_WORKERS": "8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var workers int
			c := newCmdWithCobraFlags(t, &workers)

			if err := bindEnv(c, testPrefix); err != nil {
				t.Fatalf("bindEnv() error = %v, want nil", err)
			}

			// Left untouched, not merely left unreported: a bound value goes
			// nowhere near the error return, but it is still a flag the command
			// now carries as set by someone.
			for _, name := range []string{"help", "version"} {
				f := c.Flags().Lookup(name)
				if f == nil {
					t.Fatalf("Lookup(%q) = nil, want the flag cobra registers", name)
				}

				if f.Changed || f.Value.String() != "false" {
					t.Errorf("--%s = %q, changed = %t; want %q and untouched", name, f.Value.String(), f.Changed, "false")
				}
			}

			// The flag beside them still resolves. What #47 asks for is an
			// exclusion of cobra's two, not of the environment.
			if workers != 8 {
				t.Errorf("workers = %d, want 8 from the environment", workers)
			}
		})
	}
}

// TestCobraAnnotatesItsOwnFlags pins the assumption bindEnv's exclusion is keyed
// on. cobra marks --help and --version as its own as it registers them, and
// reads that annotation itself to tell its flags from a command's — but nothing
// a compiler can see says the next cobra still will, and if one stopped, the
// only symptom would be #47 returning: STRESSY_VERSION failing runs again. A
// dependency bump that drops it has to fail here instead.
func TestCobraAnnotatesItsOwnFlags(t *testing.T) {
	var workers int
	c := newCmdWithCobraFlags(t, &workers)

	for _, name := range []string{"help", "version"} {
		f := c.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("Lookup(%q) = nil, want cobra to have registered its flag", name)
		}

		if !setByCobra(f) {
			t.Errorf("--%s carries no %s annotation, which is how bindEnv knows to leave it alone (#47)", name, cobra.FlagSetByCobraAnnotation)
		}
	}

	// And the command's own flag must not carry it, or the exclusion would take
	// every flag stressy declares with it and the environment would configure
	// nothing at all.
	f := c.Flags().Lookup("workers")
	if f == nil {
		t.Fatal(`Lookup("workers") = nil, want the registered flag`)
	}

	if setByCobra(f) {
		t.Errorf("--workers carries the %s annotation, so the exclusion would swallow stressy's own flags", cobra.FlagSetByCobraAnnotation)
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
