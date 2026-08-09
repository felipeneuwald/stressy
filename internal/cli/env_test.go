package cli

import (
	"flag"
	"io"
	"strings"
	"testing"
)

// testPrefix rather than STRESSY, so an ambient STRESSY_* cannot reach a case.
const testPrefix = "FLAGTEST"

// newIntSetting builds a bare flag set carrying a single int-valued setting,
// and the one-row table bindEnv reads it through. workersValue rather than
// flag.IntVar, because bindEnv names the variable and the Value names the bad
// value, and stock IntVar reports a bare "parse error" that names neither.
func newIntSetting(t *testing.T, name string, p *int, defaultValue int) (*flag.FlagSet, []setting) {
	t.Helper()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	value := newWorkersValue(defaultValue, p)
	fs.Var(value, name, "number of workers")

	return fs, []setting{{long: name, usage: "number of workers", env: true, value: value}}
}

func TestBindEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		commandLine string
		want        int
		// wantSource is what the flag set's own record of what was set cannot
		// answer: which variable a value arrived through (#51).
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
			fs, flags := newIntSetting(t, "workers", &workers, 1)

			if tt.commandLine != "" {
				if err := fs.Parse([]string{"-workers", tt.commandLine}); err != nil {
					t.Fatalf("Parse() error = %v, want nil", err)
				}
			}

			fromEnv, err := bindEnv(fs, flags, testPrefix)
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
	fs, flags := newIntSetting(t, "workers", &workers, 1)

	fromEnv, err := bindEnv(fs, flags, testPrefix)
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

// TestBindEnvSkipsWhatTheEnvironmentCannotFill covers #47: STRESSY_VERSION=0.4.0
// aborted every run. Nothing marks --help and --version now that stressy
// registers them itself, so the whole of the exclusion is the table row.
func TestBindEnvSkipsWhatTheEnvironmentCannotFill(t *testing.T) {
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

			var (
				workers           int
				help, showVersion bool
			)

			fs, flags := newIntSetting(t, "workers", &workers, 1)
			flags = append(flags,
				setting{long: "help", short: "h", value: newBoolValue(&help)},
				setting{long: "version", short: "v", value: newBoolValue(&showVersion)},
			)

			fromEnv, err := bindEnv(fs, flags, testPrefix)
			if err != nil {
				t.Fatalf("bindEnv() error = %v, want nil", err)
			}

			for _, name := range []string{"help", "version"} {
				if source, ok := fromEnv[name]; ok {
					t.Errorf("bindEnv() reports --%s as filled from %q, want a setting the environment cannot fill left out entirely", name, source)
				}
			}

			if help || showVersion {
				t.Errorf("--help, --version = %t, %t; want both untouched", help, showVersion)
			}

			if workers != 8 {
				t.Errorf("workers = %d, want 8 from the environment", workers)
			}
		})
	}
}

// TestBindEnvResolvesShorthands: Visit reports the spelling that was typed, so
// `-w 4` marks `w`. Unresolved, FLAGTEST_WORKERS would overwrite it (#51).
func TestBindEnvResolvesShorthands(t *testing.T) {
	t.Setenv("FLAGTEST_WORKERS", "8")

	var workers int

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	value := newWorkersValue(1, &workers)
	fs.Var(value, "workers", "number of workers")
	fs.Var(value, "w", "number of workers")

	flags := []setting{{long: "workers", short: "w", env: true, value: value}}

	if err := fs.Parse([]string{"-w", "4"}); err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	fromEnv, err := bindEnv(fs, flags, testPrefix)
	if err != nil {
		t.Fatalf("bindEnv() error = %v, want nil", err)
	}

	if workers != 4 {
		t.Errorf("workers = %d, want the 4 the shorthand set", workers)
	}

	if source, ok := fromEnv["workers"]; ok {
		t.Errorf("bindEnv() reports workers as filled from %q, want a flag given on the command line left alone", source)
	}
}

// TestBindEnvMultiWordFlagName is what envName's dash substitution exists for.
func TestBindEnvMultiWordFlagName(t *testing.T) {
	t.Setenv("FLAGTEST_MAX_RETRIES", "3")

	var retries int
	fs, flags := newIntSetting(t, "max-retries", &retries, 0)

	fromEnv, err := bindEnv(fs, flags, testPrefix)
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
