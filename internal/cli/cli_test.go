package cli

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// newTestCmd builds the real command with a no-op RunE, output discarded. A
// case setting its own STRESSY_* value must do so after this returns.
func newTestCmd(t *testing.T, cfg *stressy.Cfg) *cobra.Command {
	t.Helper()

	clearStressyEnv(t)

	c := NewCmd(cfg)
	c.RunE = func(*cobra.Command, []string) error { return nil }

	c.SilenceErrors = true

	c.SetOut(io.Discard)
	c.SetErr(io.Discard)

	return c
}

// clearStressyEnv blanks every variable a run can be configured from (#58).
func clearStressyEnv(t *testing.T) {
	t.Helper()

	NewCmd(&stressy.Cfg{}).Flags().VisitAll(func(f *pflag.Flag) {
		t.Setenv(envName(envPrefix, f.Name), "")
	})
}

// TestBoundedRunExecutesTheStressTest runs the real RunE, the join to the engine (#53).
func TestBoundedRunExecutesTheStressTest(t *testing.T) {
	const (
		timeout = 50 * time.Millisecond
		// As loose as internal/stressy's stopBudget, and for its reasons.
		budget = 30 * time.Second
	)

	clearStressyEnv(t)

	var cfg stressy.Cfg
	cmd := NewCmd(&cfg)
	cmd.SetArgs([]string{"-w", "1", "-t", timeout.String()})

	start := time.Now()

	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute() error = %v, want a completed run", err)
		}
	case <-time.After(budget):
		t.Fatalf("Execute() did not return within %s of a run given %s", budget, timeout)
	}

	if elapsed := time.Since(start); elapsed < timeout {
		t.Errorf("Execute() returned after %s, want at least the %s the run was given", elapsed, timeout)
	}

	if cfg.Workers != 1 || cfg.Timeout != timeout {
		t.Errorf("Workers, Timeout = %d, %s; want 1, %s", cfg.Workers, cfg.Timeout, timeout)
	}
}

// TestFlagRegistration pins what `stressy --help` prints for every flag.
func TestFlagRegistration(t *testing.T) {
	tests := []struct {
		name      string
		shorthand string
		flagType  string
		defValue  string
		wantUsage []string
	}{
		{name: "workers", shorthand: "w", flagType: "int", defValue: strconv.Itoa(defaultWorkers()), wantUsage: []string{"parallel workers"}},
		// An int-of-seconds description gives no reason to try a duration (#26).
		{name: "timeout", shorthand: "t", flagType: "duration", defValue: "0s", wantUsage: []string{"5m", "seconds"}},
		{name: "report", shorthand: "r", flagType: "duration", defValue: "0s", wantUsage: []string{"5m", "seconds"}},
	}

	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := cmd.Flags().Lookup(tt.name)
			if f == nil {
				t.Fatalf("Lookup(%q) = nil, want the registered flag", tt.name)
			}

			if f.Shorthand != tt.shorthand {
				t.Errorf("%s shorthand = %q, want %q", tt.name, f.Shorthand, tt.shorthand)
			}
			if f.Value.Type() != tt.flagType {
				t.Errorf("%s type = %q, want %q", tt.name, f.Value.Type(), tt.flagType)
			}
			if f.DefValue != tt.defValue {
				t.Errorf("%s default = %q, want %q", tt.name, f.DefValue, tt.defValue)
			}

			for _, fragment := range tt.wantUsage {
				if !strings.Contains(f.Usage, fragment) {
					t.Errorf("%s usage = %q, want it to contain %q", tt.name, f.Usage, fragment)
				}
			}
		})
	}
}

// TestConfigResolution covers what a run is configured with, not how durations parse.
func TestConfigResolution(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		args        []string
		wantWorkers int
		wantTimeout time.Duration
		wantReport  time.Duration
	}{
		{
			name:        "flags",
			args:        []string{"-w", "4", "-t", "30", "-r", "15s"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
			wantReport:  15 * time.Second,
		},
		{
			name:        "environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			wantWorkers: 8,
			wantTimeout: 60 * time.Second,
		},
		{
			name:        "flags take precedence over environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			args:        []string{"-w", "2", "-t", "5"},
			wantWorkers: 2,
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "environment fills in only the unset flag",
			env:         map[string]string{"STRESSY_WORKERS": "8"},
			args:        []string{"-t", "5"},
			wantWorkers: 8,
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "every setting from the environment at once",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60", "STRESSY_REPORT": "10s"},
			wantWorkers: 8,
			wantTimeout: 60 * time.Second,
			wantReport:  10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}

			if cfg.Workers != tt.wantWorkers {
				t.Errorf("Workers = %d, want %d", cfg.Workers, tt.wantWorkers)
			}
			if cfg.Timeout != tt.wantTimeout {
				t.Errorf("Timeout = %s, want %s", cfg.Timeout, tt.wantTimeout)
			}
			if cfg.Report != tt.wantReport {
				t.Errorf("Report = %s, want %s", cfg.Report, tt.wantReport)
			}
		})
	}
}

// malformedValues drive both the command-line path and the environment one.
var malformedValues = []struct {
	name  string
	flag  string
	env   string
	other []string // a valid setting for the flag not under test
	value string
	want  string
}{
	{name: "timeout", flag: "-t", env: "STRESSY_TIMEOUT", other: []string{"-w", "1"}, value: "not-a-number", want: "want a duration such as 30s or 5m"},
	{name: "report", flag: "-r", env: "STRESSY_REPORT", other: []string{"-w", "1", "-t", "100ms"}, value: "not-a-number", want: "want a duration such as 30s or 5m"},
	{name: "workers", flag: "-w", env: "STRESSY_WORKERS", other: []string{"-t", "100ms"}, value: "abc", want: "want a whole number"},
	{name: "workers, a float", flag: "-w", env: "STRESSY_WORKERS", other: []string{"-t", "100ms"}, value: "2.0", want: "want a whole number"},
	{name: "workers, past what an int holds", flag: "-w", env: "STRESSY_WORKERS", other: []string{"-t", "100ms"}, value: "99999999999999999999", want: "out of range"},
}

// TestMalformedEnvValueIsRejected covers #10: a swallowed STRESSY_TIMEOUT ran forever.
func TestMalformedEnvValueIsRejected(t *testing.T) {
	for _, tt := range malformedValues {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			t.Setenv(tt.env, tt.value)

			cmd.SetArgs(tt.other)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() with %s=%s error = nil, want the malformed value to be rejected", tt.env, tt.value)
			}

			checkMalformedMessage(t, err, tt.env, tt.value, tt.want)
		})
	}
}

// TestMalformedFlagValueIsRejected is the command-line counterpart (#50).
func TestMalformedFlagValueIsRejected(t *testing.T) {
	for _, tt := range malformedValues {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)
			cmd.SetArgs(append([]string{tt.flag, tt.value}, tt.other...))

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute(%s %s) error = nil, want a parse error", tt.flag, tt.value)
			}

			checkMalformedMessage(t, err, tt.flag, tt.value, tt.want)
		})
	}
}

// checkMalformedMessage: where the value came from, which one, and what is valid.
func checkMalformedMessage(t *testing.T, err error, source, value, want string) {
	t.Helper()

	for _, fragment := range []string{source, value, want} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error = %q, want it to contain %q", err, fragment)
		}
	}

	if strings.Contains(err.Error(), "strconv") {
		t.Errorf("error = %q, want no strconv internals in it (#50)", err)
	}
}

// TestFlagsCobraSetItselfAreNotConfigurable covers #47: STRESSY_VERSION aborted runs.
func TestFlagsCobraSetItselfAreNotConfigurable(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "values no bool flag can parse", env: map[string]string{"STRESSY_HELP": "yes", "STRESSY_VERSION": "0.4.0"}},
		{name: "values a bool flag can parse", env: map[string]string{"STRESSY_HELP": "true", "STRESSY_VERSION": "true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var ran bool
			cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
			cmd.SetArgs([]string{"-w", "1", "-t", "100ms"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v, want the run the operator asked for", err)
			}

			if !ran {
				t.Error("RunE was not called, want the stress test to run")
			}

			if cfg.Workers != 1 || cfg.Timeout != 100*time.Millisecond {
				t.Errorf("Workers, Timeout = %d, %s; want 1, 100ms", cfg.Workers, cfg.Timeout)
			}
		})
	}
}

// TestFlagsCobraSetItselfWorkOnTheCommandLine is #47's other half: the flags still work.
func TestFlagsCobraSetItselfWorkOnTheCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version", args: []string{"--version"}, want: "version " + version},
		{name: "version shorthand", args: []string{"-v"}, want: "version " + version},
		{name: "help", args: []string{"--help"}, want: "Usage:"},
		{name: "help shorthand", args: []string{"-h"}, want: "Usage:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			var ran bool
			cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute(%q) error = %v, want nil", tt.args, err)
			}

			if ran {
				t.Errorf("Execute(%q) ran the stress test, want cobra to have answered and returned", tt.args)
			}

			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("Execute(%q) printed %q, want it to contain %q", tt.args, out.String(), tt.want)
			}
		})
	}
}

// TestWorkersDefaultsToUsableCPUs covers #24: NumCPU ignores a cgroup quota.
func TestWorkersDefaultsToUsableCPUs(t *testing.T) {
	want := 3
	if runtime.NumCPU() == want {
		// Otherwise NumCPU would satisfy this too and the case proves nothing.
		want = 2
	}

	prev := runtime.GOMAXPROCS(want)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	if cfg.Workers != want {
		t.Errorf("Workers = %d with GOMAXPROCS at %d and NumCPU at %d, want %d", cfg.Workers, want, runtime.NumCPU(), want)
	}
	if cfg.Timeout != 0 {
		t.Errorf("Timeout = %s with nothing setting it, want 0", cfg.Timeout)
	}
	if cfg.Report != 0 {
		t.Errorf("Report = %s with nothing setting it, want 0", cfg.Report)
	}
}

// TestPositionalArgumentsAreRejected covers #17b: `stressy 4` ignored the 4.
func TestPositionalArgumentsAreRejected(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantArg string
	}{
		{name: "bare argument", args: []string{"4"}, wantArg: "4"},
		{name: "several arguments", args: []string{"foo", "bar", "4"}, wantArg: "foo"},
		{name: "argument after a flag", args: []string{"-w", "4", "extra"}, wantArg: "extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute(%q) error = nil, want the argument to be rejected", tt.args)
			}

			if !strings.Contains(err.Error(), tt.wantArg) {
				t.Errorf("Execute(%q) error = %q, want it to name %q", tt.args, err, tt.wantArg)
			}
		})
	}
}

// TestUsageIsSilencedOnlyForRuntimeErrors covers #17a, and the exception: a
// mistyped flag name still wants the flag list.
func TestUsageIsSilencedOnlyForRuntimeErrors(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		args        []string
		runErr      error
		wantSilence bool
	}{
		{name: "unknown flag", args: []string{"--bogus"}},
		{name: "invalid flag value", args: []string{"-t", "not-a-number"}},
		{name: "positional argument", args: []string{"4"}},
		{name: "rejected environment variable", env: map[string]string{"STRESSY_TIMEOUT": "not-a-number"}, wantSilence: true},
		// Why parseWorkers checks no range: pflag rejecting it means usage (#17a).
		{name: "out-of-range flag value", args: []string{"-w", "0"}, wantSilence: true},
		{name: "out-of-range environment variable", env: map[string]string{"STRESSY_WORKERS": "0"}, args: []string{"-t", "100ms"}, wantSilence: true},
		{name: "runtime failure", runErr: errors.New("workers must be 1 or greater"), wantSilence: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			cmd.RunE = func(*cobra.Command, []string) error { return tt.runErr }
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want the case to fail")
			}

			// The behaviour, not the field a hand-printed screen would leave green (#60).
			wantUsage := !tt.wantSilence
			if gotUsage := strings.Contains(out.String(), "Usage:"); gotUsage != wantUsage {
				t.Errorf("Execute(%q) printed:\n%s\nwant the usage screen printed = %t", tt.args, out.String(), wantUsage)
			}

			if cmd.SilenceUsage != tt.wantSilence {
				t.Errorf("SilenceUsage = %t, want %t", cmd.SilenceUsage, tt.wantSilence)
			}
		})
	}
}
