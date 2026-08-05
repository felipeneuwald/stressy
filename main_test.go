package main

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

// newTestCmd builds the real command over the given configuration, but with a
// no-op RunE: the actual RunE starts the stress test, which saturates the CPU
// and blocks until signalled. Everything else — flag registration, the
// environment binding in PreRunE — is what main() runs.
// TestBoundedRunExecutesTheStressTest is the one case that keeps the real one.
//
// A case that sets its own STRESSY_* value has to do so after this returns:
// clearStressyEnv runs here, and it blanks them. The environment is read at
// Execute time, so that ordering costs nothing.
func newTestCmd(t *testing.T, cfg *stressy.Cfg) *cobra.Command {
	t.Helper()

	clearStressyEnv(t)

	c := newCmd(cfg)
	c.RunE = func(*cobra.Command, []string) error { return nil }

	// Output goes nowhere: these cases read the error Execute returns, and a
	// failing one would otherwise print its message and, for a usage error,
	// the whole help screen into the test output. SilenceUsage is deliberately
	// left as newCmd sets it, since TestUsageIsSilencedOnlyForRuntimeErrors
	// asserts on exactly that.
	c.SilenceErrors = true

	c.SetOut(io.Discard)
	c.SetErr(io.Discard)

	return c
}

// clearStressyEnv blanks every variable a run can be configured from, so that
// these cases read the configuration in front of them rather than the one the
// shell running `go test` happens to export. `STRESSY_WORKERS=4 go test .`
// failed six cases in this file, on a machine that had done nothing wrong but
// use the tool (#58).
//
// The case below that runs real workers calls it too, though it pins both flags
// on the command line and so cannot be reached from the environment as written:
// there an ambient STRESSY_TIMEOUT would be worse than a failure, and this is
// what keeps that true of any later case that stops passing `-t`.
//
// Blank rather than unset, which is the same move docs_test.go was already
// making: bindEnv treats an empty value as unset deliberately, because
// `STRESSY_WORKERS=${WORKERS}` with WORKERS undefined is a common shape in
// compose files and pod specs, and t.Setenv restores what was there afterwards.
//
// The names are read off the command's own flags through envName, the mapping
// the binary itself uses, so a third flag is covered by this the day it is
// registered rather than the day someone remembers this function exists.
func clearStressyEnv(t *testing.T) {
	t.Helper()

	newCmd(&stressy.Cfg{}).Flags().VisitAll(func(f *pflag.Flag) {
		t.Setenv(envName(envPrefix, f.Name), "")
	})
}

// TestBoundedRunExecutesTheStressTest is the one case here that runs the
// command main() builds with the RunE main() runs, rather than the stub
// newTestCmd swaps in. That single line — `stressy.New(*cfg).Run()` — is the
// whole of the join between the CLI and the engine, and it was executed by
// nothing: a refactor that stopped threading cfg through it, or built RunE over
// a zero Cfg, would break every real invocation of this tool while the suite
// stayed green (#53).
//
// Bounded on purpose, and cheaply: one worker for 50ms, plus the hash that
// worker is inside when the deadline lands — ~0.18s on an M-series core, and
// under 2s on a loaded CI runner under -race. TestExitCodes covers the same
// wiring through a real process; this covers it on the platforms that test is
// not built for, and reads the return value that one can only see as a status.
func TestBoundedRunExecutesTheStressTest(t *testing.T) {
	const (
		timeout = 50 * time.Millisecond
		// budget bounds the case end to end, and is as loose as
		// internal/stressy's stopBudget for the same reason: the hash a worker
		// is inside measures ~0.18s on an M-series core and ~1.9s on a loaded
		// CI runner under -race. It is reached only by the regression this case
		// exists for — a `-t` that never arrives is an indefinite run, and
		// without this that is a hang until the whole binary's timeout fires
		// rather than a failure that names itself.
		budget = 30 * time.Second
	)

	clearStressyEnv(t)

	var cfg stressy.Cfg
	cmd := newCmd(&cfg)
	cmd.SetArgs([]string{"-w", "1", "-t", timeout.String()})

	start := time.Now()

	// Run prints its own lines with fmt.Println to os.Stdout rather than
	// through cobra's writer, so they land in the test output. Left there: they
	// are what TestExitCodes reads off a child process, and seeing them here is
	// evidence this case ran the real thing.
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

	// What makes this more than "Execute returned nil": a RunE handed a stale or
	// half-populated Cfg — one the `-t` never reached — returns nil as well,
	// having run for some length of time other than the one asked for.
	if elapsed := time.Since(start); elapsed < timeout {
		t.Errorf("Execute() returned after %s, want at least the %s the run was given", elapsed, timeout)
	}

	if cfg.Workers != 1 || cfg.Timeout != timeout {
		t.Errorf("Workers, Timeout = %d, %s; want 1, %s", cfg.Workers, cfg.Timeout, timeout)
	}
}

// TestFlagRegistration pins the operator-facing shape of both flags: name,
// shorthand, type placeholder and default are what `stressy --help` prints,
// and the shorthands are the spelling every existing command line uses.
func TestFlagRegistration(t *testing.T) {
	tests := []struct {
		name      string
		shorthand string
		flagType  string
		defValue  string
	}{
		{name: "workers", shorthand: "w", flagType: "int", defValue: strconv.Itoa(defaultWorkers())},
		{name: "timeout", shorthand: "t", flagType: "duration", defValue: "0s"},
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
			if f.Usage == "" {
				t.Errorf("%s usage = %q, want help text", tt.name, f.Usage)
			}
		})
	}
}

// TestConfigResolution covers what a run is configured with, from flags, from
// the environment, and from neither. The cases that expect defaultWorkers()
// are the ones that pass no worker count: since #24 that default is the
// machine's, so a literal here would pin the test to whatever CPU count it
// happened to be written on.
func TestConfigResolution(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		args        []string
		wantWorkers int
		wantTimeout time.Duration
	}{
		{
			name:        "defaults",
			wantWorkers: defaultWorkers(),
			wantTimeout: 0,
		},
		{
			name:        "long flags",
			args:        []string{"--workers", "4", "--timeout", "30"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "short flags",
			args:        []string{"-w", "4", "-t", "30"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
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
			name:        "duration flag",
			args:        []string{"--timeout", "5m"},
			wantWorkers: defaultWorkers(),
			wantTimeout: 5 * time.Minute,
		},
		{
			name:        "compound duration flag",
			args:        []string{"-t", "1h30m"},
			wantWorkers: defaultWorkers(),
			wantTimeout: 90 * time.Minute,
		},
		// #26: STRESSY_TIMEOUT=60s is the natural thing to type, and before
		// --timeout took a duration it was the sharpest edge of #10 — the value
		// failed to parse, and the run that was meant to last a minute ran
		// forever and exited 0.
		{
			name:        "duration environment variable",
			env:         map[string]string{"STRESSY_TIMEOUT": "60s"},
			wantWorkers: defaultWorkers(),
			wantTimeout: 60 * time.Second,
		},
		{
			name:        "sub-second duration",
			args:        []string{"-t", "250ms"},
			wantWorkers: defaultWorkers(),
			wantTimeout: 250 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built, not before: newTestCmd blanks the
			// ambient STRESSY_* variables, and a case's own values have to
			// survive that. See clearStressyEnv.
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
		})
	}
}

// malformedValues are values neither flag can parse, each with the hint its
// rejection has to carry. A run is configured from the same two parsers however
// it is spelled, so one table drives both the command-line path and the
// environment one.
var malformedValues = []struct {
	name string
	flag string
	env  string
	// other is a valid setting for the flag not under test, so that the value
	// under test is the only thing wrong with the run.
	other []string
	value string
	want  string
}{
	{
		name:  "timeout",
		flag:  "-t",
		env:   "STRESSY_TIMEOUT",
		other: []string{"-w", "1"},
		value: "not-a-number",
		want:  "want a duration such as 30s or 5m",
	},
	// #50: these three used to end in `strconv.ParseInt: parsing "abc":
	// invalid syntax`, which names a Go standard library function and no valid
	// value — a sharp inconsistency with the flag above, whose parser was
	// hand-written precisely to avoid it.
	{
		name:  "workers",
		flag:  "-w",
		env:   "STRESSY_WORKERS",
		other: []string{"-t", "100ms"},
		value: "abc",
		want:  "want a whole number",
	},
	{
		name:  "workers, a float",
		flag:  "-w",
		env:   "STRESSY_WORKERS",
		other: []string{"-t", "100ms"},
		value: "2.0",
		want:  "want a whole number",
	},
	{
		name:  "workers, past what an int holds",
		flag:  "-w",
		env:   "STRESSY_WORKERS",
		other: []string{"-t", "100ms"},
		value: "99999999999999999999",
		want:  "out of range",
	},
}

// TestMalformedEnvValueIsRejected covers issue #10: a non-numeric
// STRESSY_TIMEOUT used to be swallowed, leaving the flag at pflag's zero value
// and turning a run the operator had bounded into an endless one, exit code 0.
// Since #50 it also holds the message to saying what a valid value looks like,
// on both variables rather than on the one that had a hand-written parser.
func TestMalformedEnvValueIsRejected(t *testing.T) {
	for _, tt := range malformedValues {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built; see clearStressyEnv.
			t.Setenv(tt.env, tt.value)

			cmd.SetArgs(tt.other)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() with %s=%s error = nil, want the malformed value to be rejected", tt.env, tt.value)
			}

			// The message has to name the offending variable and value, or an
			// operator staring at a failed run has nothing to go on — and it
			// has to say what a valid value would have been, or they have
			// nothing to correct it to.
			checkMalformedMessage(t, err, tt.env, tt.value, tt.want)
		})
	}
}

// TestMalformedFlagValueIsRejected is the command-line counterpart: pflag
// parses these itself, so this path reported the error even before #10 — in
// strconv's words for one of the two flags, which is what #50 is about.
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

			// pflag names the flag itself, in both spellings.
			checkMalformedMessage(t, err, tt.flag, tt.value, tt.want)
		})
	}
}

// checkMalformedMessage holds a rejected value's message to the three things it
// owes the reader: where the value came from, which value it was, and what a
// valid one looks like — in words rather than in strconv's.
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

// TestFlagsCobraSetItselfAreNotConfigurable covers #47 through the command
// main() runs. --help and --version exist only because Execute registers them,
// so this is the level the bug lived at: STRESSY_VERSION is a plausible name
// for an image-tag pin in a compose file or a CI environment, and one leaking
// into the container aborted every run with exit 1 over a flag the operator
// never touched. Both spellings are covered — the value that failed to parse
// and the value that parsed and was silently discarded.
func TestFlagsCobraSetItselfAreNotConfigurable(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "values no bool flag can parse",
			env:  map[string]string{"STRESSY_HELP": "yes", "STRESSY_VERSION": "0.4.0"},
		},
		{
			name: "values a bool flag can parse",
			env:  map[string]string{"STRESSY_HELP": "true", "STRESSY_VERSION": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built; see clearStressyEnv. These two are
			// not among the variables it blanks — they are cobra's flags, which
			// is the point of the case — but the ordering is the same one every
			// case in this file keeps.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var ran bool
			cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
			cmd.SetArgs([]string{"-w", "1", "-t", "100ms"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v, want the run the operator asked for", err)
			}

			// A run that starts, with the configuration the command line gave
			// it: "no error" alone would also pass on a build where the value
			// had taken effect and printed help instead of running anything.
			if !ran {
				t.Error("RunE was not called, want the stress test to run")
			}

			if cfg.Workers != 1 || cfg.Timeout != 100*time.Millisecond {
				t.Errorf("Workers, Timeout = %d, %s; want 1, 100ms", cfg.Workers, cfg.Timeout)
			}
		})
	}
}

// TestFlagsCobraSetItselfWorkOnTheCommandLine is the other half of #47: what
// the fix takes away is the environment's reach into --help and --version, not
// the flags. Both are cobra's to act on, both print and return before RunE, and
// neither had a test holding it to that.
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

			// newTestCmd discards output; these cases are about what gets
			// printed, so they read it back instead.
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

// TestWorkersDefaultsToUsableCPUs covers #24. A CPU stress tool that loads one
// core unless told otherwise is a surprising default, and the fix has to be
// GOMAXPROCS rather than NumCPU or it is worse in a container than what it
// replaced: NumCPU ignores the cgroup CPU quota, so a 64-core node under
// `limits.cpu: 2` would start 64 workers to be throttled into 2 cores' worth
// of bursty, unrepresentative load.
//
// Moving GOMAXPROCS away from NumCPU is what separates the two candidates:
// only one of them follows.
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
}

// TestPositionalArgumentsAreRejected covers #17b. stressy has never read an
// operand, and cobra's default is to accept and discard them — so `stressy 4`,
// a reasonable guess at the worker count, ran the default number of workers
// and said nothing about the 4.
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

			// cobra.NoArgs would report `unknown command "4"`, which sends a
			// user of a command with no subcommands looking for one. The
			// message has to name what was rejected instead.
			if !strings.Contains(err.Error(), tt.wantArg) {
				t.Errorf("Execute(%q) error = %q, want it to name %q", tt.args, err, tt.wantArg)
			}
		})
	}
}

// TestUsageIsSilencedOnlyForRuntimeErrors covers #17a. Every failure used to
// print the entire help screen after its message, so `stressy -w 0` answered a
// one-line configuration mistake with a page of flags. The fix has to keep
// usage on the paths where it is the answer: a user who mistypes a flag name
// wants the flag list.
//
// cobra parses flags and validates operands before PreRunE runs, which is what
// makes the split available: anything failing from PreRunE on is a runtime
// error by construction.
func TestUsageIsSilencedOnlyForRuntimeErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		// runErr is returned from RunE, standing in for the stress test's own
		// validation — the real RunE would saturate the CPU.
		runErr      error
		wantSilence bool
	}{
		{
			name:        "unknown flag",
			args:        []string{"--bogus"},
			wantSilence: false,
		},
		{
			name:        "invalid flag value",
			args:        []string{"-t", "not-a-number"},
			wantSilence: false,
		},
		{
			name:        "positional argument",
			args:        []string{"4"},
			wantSilence: false,
		},
		{
			// A bad STRESSY_TIMEOUT is a configuration error, not a spelling
			// one, and the message already names the variable and the value.
			name:        "rejected environment variable",
			env:         map[string]string{"STRESSY_TIMEOUT": "not-a-number"},
			wantSilence: true,
		},
		{
			// The case #17a is named after, and the reason parseWorkers checks
			// no range: an out-of-range value that failed in pflag's parser
			// would be a usage error by construction, and `-w 0` would answer a
			// one-line mistake with a page of flags again. It fails in
			// validateRanges instead, which runs after the line above.
			name:        "out-of-range flag value",
			args:        []string{"-w", "0"},
			wantSilence: true,
		},
		{
			name:        "out-of-range environment variable",
			env:         map[string]string{"STRESSY_WORKERS": "0"},
			args:        []string{"-t", "100ms"},
			wantSilence: true,
		},
		{
			name:        "runtime failure",
			runErr:      errors.New("workers must be 1 or greater"),
			wantSilence: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built; see clearStressyEnv.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			// newTestCmd discards both streams. What an operator sees is the
			// whole of what #17a is about, so this case reads it back instead —
			// one buffer for both, because which stream cobra routes the usage
			// screen to is exactly the internal detail this should not depend
			// on. SilenceErrors, which newTestCmd sets, suppresses the error
			// line rather than the usage screen, so it leaves this alone.
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)

			cmd.RunE = func(*cobra.Command, []string) error { return tt.runErr }
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want the case to fail")
			}

			// The behaviour, rather than the field that currently produces it:
			// a usage screen printed by hand on some error path, or a cobra
			// release that honours SilenceUsage differently, regresses what the
			// operator reads while leaving the field — and a test asserting
			// only the field — green (#60).
			wantUsage := !tt.wantSilence
			if gotUsage := strings.Contains(out.String(), "Usage:"); gotUsage != wantUsage {
				t.Errorf("Execute(%q) printed:\n%s\nwant the usage screen printed = %t", tt.args, out.String(), wantUsage)
			}

			// Kept as the second assertion: it is the mechanism, and it says so
			// directly when the line above fails.
			if cmd.SilenceUsage != tt.wantSilence {
				t.Errorf("SilenceUsage = %t, want %t", cmd.SilenceUsage, tt.wantSilence)
			}
		})
	}
}

// TestTimeoutHelpAdvertisesDuration guards the operator-facing half of #26: a
// timeout that accepts "5m" but still describes itself as an int of seconds
// leaves the reader no reason to try the duration form.
func TestTimeoutHelpAdvertisesDuration(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatal(`Lookup("timeout") = nil, want the registered flag`)
	}

	if f.Value.Type() != "duration" {
		t.Errorf("timeout type = %q, want %q", f.Value.Type(), "duration")
	}
	if !strings.Contains(f.Usage, "5m") {
		t.Errorf("timeout usage = %q, want it to show the duration form", f.Usage)
	}
	if !strings.Contains(f.Usage, "seconds") {
		t.Errorf("timeout usage = %q, want it to state that a bare number is seconds", f.Usage)
	}
}
