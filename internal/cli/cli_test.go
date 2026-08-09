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

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// newTestCmd builds the real command with the stress test stubbed out and its
// output discarded. A case setting its own STRESSY_* value must do so after
// this returns.
func newTestCmd(t *testing.T, cfg *stressy.Cfg) *command {
	t.Helper()

	clearStressyEnv(t)

	c := newCmd(cfg)
	c.run = func(*stressy.Cfg) error { return nil }
	c.stdout, c.stderr = io.Discard, io.Discard

	return c
}

// clearStressyEnv blanks every variable a run can be configured from (#58).
func clearStressyEnv(t *testing.T) {
	t.Helper()

	for _, s := range Settings() {
		if s.EnvVar != "" {
			t.Setenv(s.EnvVar, "")
		}
	}
}

// TestBoundedRunExecutesTheStressTest runs the real run, the join to the engine (#53).
func TestBoundedRunExecutesTheStressTest(t *testing.T) {
	const (
		timeout = 50 * time.Millisecond
		// As loose as internal/stressy's stopBudget, and for its reasons.
		budget = 30 * time.Second
	)

	clearStressyEnv(t)

	var cfg stressy.Cfg
	cmd := newCmd(&cfg)
	cmd.stderr = io.Discard

	start := time.Now()

	done := make(chan error, 1)
	go func() { done <- cmd.execute([]string{"-w", "1", "-t", timeout.String()}) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execute() error = %v, want a completed run", err)
		}
	case <-time.After(budget):
		t.Fatalf("execute() did not return within %s of a run given %s", budget, timeout)
	}

	if elapsed := time.Since(start); elapsed < timeout {
		t.Errorf("execute() returned after %s, want at least the %s the run was given", elapsed, timeout)
	}

	if cfg.Workers != 1 || cfg.Timeout != timeout {
		t.Errorf("Workers, Timeout = %d, %s; want 1, %s", cfg.Workers, cfg.Timeout, timeout)
	}
}

// TestFlagRegistration pins what `stressy --help` prints for every flag.
func TestFlagRegistration(t *testing.T) {
	tests := []struct {
		name        string
		shorthand   string
		placeholder string
		def         string
		wantUsage   []string
	}{
		{name: "workers", shorthand: "w", placeholder: "int", def: strconv.Itoa(defaultWorkers()), wantUsage: []string{"parallel workers"}},
		// An int-of-seconds description gives no reason to try a duration (#26).
		{name: "timeout", shorthand: "t", placeholder: "duration", def: "0s", wantUsage: []string{"5m", "seconds"}},
		{name: "report", shorthand: "r", placeholder: "duration", def: "0s", wantUsage: []string{"5m", "seconds"}},
	}

	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := lookupSetting(cmd, tt.name)
			if !ok {
				t.Fatalf("the flag table has no %q row, want the registered flag", tt.name)
			}

			if s.short != tt.shorthand {
				t.Errorf("%s shorthand = %q, want %q", tt.name, s.short, tt.shorthand)
			}
			if s.placeholder != tt.placeholder {
				t.Errorf("%s placeholder = %q, want %q", tt.name, s.placeholder, tt.placeholder)
			}
			if s.def != tt.def {
				t.Errorf("%s default = %q, want %q", tt.name, s.def, tt.def)
			}

			for _, fragment := range tt.wantUsage {
				if !strings.Contains(s.usage, fragment) {
					t.Errorf("%s usage = %q, want it to contain %q", tt.name, s.usage, fragment)
				}
			}
		})
	}
}

// TestBothSpellingsShareOneValue is what makes `-w 4`, `--workers 4`,
// `-workers 4` and `--w 4` one setting rather than two the flag package would
// otherwise keep apart — and what bindEnv's short-to-long resolution rests on.
func TestBothSpellingsShareOneValue(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	for _, s := range cmd.flags {
		long, short := cmd.fs.Lookup(s.long), cmd.fs.Lookup(s.short)
		if long == nil || short == nil {
			t.Fatalf("--%s is registered as %v and -%s as %v, want both", s.long, long, s.short, short)
		}

		if long.Value != short.Value {
			t.Errorf("--%s and -%s hold different values, so one of the two spellings writes somewhere nothing reads", s.long, s.short)
		}
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
			// Every spelling one Var pair admits, on one command line.
			name:        "every spelling of a flag",
			args:        []string{"--workers", "4", "-timeout", "30", "--r", "15s"},
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
			// The shorthand marks `w`, not `workers`; unresolved, the variable
			// would win a precedence contest it is meant to lose.
			name:        "a shorthand takes precedence over its environment variable",
			env:         map[string]string{"STRESSY_WORKERS": "8"},
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

			if err := cmd.execute(tt.args); err != nil {
				t.Fatalf("execute() error = %v, want nil", err)
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

			err := cmd.execute(tt.other)
			if err == nil {
				t.Fatalf("execute() with %s=%s error = nil, want the malformed value to be rejected", tt.env, tt.value)
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

			err := cmd.execute(append([]string{tt.flag, tt.value}, tt.other...))
			if err == nil {
				t.Fatalf("execute(%s %s) error = nil, want a parse error", tt.flag, tt.value)
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

// TestHelpAndVersionAreNotConfigurable covers #47: STRESSY_VERSION aborted runs.
// stressy registers both flags itself now, so nothing marks them as off limits
// but bindEnv walking the table rather than the flag set.
func TestHelpAndVersionAreNotConfigurable(t *testing.T) {
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
			cmd.run = func(*stressy.Cfg) error { ran = true; return nil }

			if err := cmd.execute([]string{"-w", "1", "-t", "100ms"}); err != nil {
				t.Fatalf("execute() error = %v, want the run the operator asked for", err)
			}

			if !ran {
				t.Error("run was not called, want the stress test to run")
			}

			if cfg.Workers != 1 || cfg.Timeout != 100*time.Millisecond {
				t.Errorf("Workers, Timeout = %d, %s; want 1, 100ms", cfg.Workers, cfg.Timeout)
			}
		})
	}
}

// TestHelpAndVersionWorkOnTheCommandLine is #47's other half: the flags still work.
func TestHelpAndVersionWorkOnTheCommandLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version", args: []string{"--version"}, want: "stressy version " + version},
		{name: "version shorthand", args: []string{"-v"}, want: "stressy version " + version},
		{name: "help", args: []string{"--help"}, want: "Usage:"},
		{name: "help shorthand", args: []string{"-h"}, want: "Usage:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &out

			var ran bool
			cmd.run = func(*stressy.Cfg) error { ran = true; return nil }

			if err := cmd.execute(tt.args); err != nil {
				t.Fatalf("execute(%q) error = %v, want nil", tt.args, err)
			}

			if ran {
				t.Errorf("execute(%q) ran the stress test, want the flag answered and nothing else", tt.args)
			}

			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("execute(%q) printed %q, want it to contain %q", tt.args, out.String(), tt.want)
			}
		})
	}
}

// TestHelpAndVersionGoToStdout: an operator piping `--help` into a pager, or
// `--version` into a variable, gets the answer rather than an empty stream.
func TestHelpAndVersionGoToStdout(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		t.Run(args[0], func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			var out, errOut bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &errOut

			if err := cmd.execute(args); err != nil {
				t.Fatalf("execute(%q) error = %v, want nil", args, err)
			}

			if out.Len() == 0 {
				t.Errorf("execute(%q) printed nothing to stdout", args)
			}
			if errOut.Len() != 0 {
				t.Errorf("execute(%q) printed %q to stderr, want the answer on stdout alone", args, errOut.String())
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

	if err := cmd.execute(nil); err != nil {
		t.Fatalf("execute() error = %v, want nil", err)
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

			err := cmd.execute(tt.args)
			if err == nil {
				t.Fatalf("execute(%q) error = nil, want the argument to be rejected", tt.args)
			}

			if !strings.Contains(err.Error(), tt.wantArg) {
				t.Errorf("execute(%q) error = %q, want it to name %q", tt.args, err, tt.wantArg)
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
		// Why parseWorkers checks no range: the parser rejecting it means usage (#17a).
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
			cmd.stdout, cmd.stderr = &out, &out

			cmd.run = func(*stressy.Cfg) error { return tt.runErr }

			err := cmd.execute(tt.args)
			if err == nil {
				t.Fatal("execute() error = nil, want the case to fail")
			}

			// The behaviour, not the field a hand-printed screen would leave green (#60).
			wantUsage := !tt.wantSilence
			if gotUsage := strings.Contains(out.String(), "Usage:"); gotUsage != wantUsage {
				t.Errorf("execute(%q) printed:\n%s\nwant the usage screen printed = %t", tt.args, out.String(), wantUsage)
			}

			// And the type it is selected by, which is the whole of the split now.
			if _, isUsage := errors.AsType[*usageError](err); isUsage == tt.wantSilence {
				t.Errorf("execute(%q) error = %q is a usage error = %t, want %t", tt.args, err, isUsage, wantUsage)
			}
		})
	}
}

// TestErrorsGoToStderr: a supervisor reading stdout for the run summary must
// not find a rejected command line there, flag list and all.
func TestErrorsGoToStderr(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	var out, errOut bytes.Buffer
	cmd.stdout, cmd.stderr = &out, &errOut

	if err := cmd.execute([]string{"-w", "2", "4"}); err == nil {
		t.Fatal("execute() error = nil, want the argument to be rejected")
	}

	if out.Len() != 0 {
		t.Errorf("execute() printed %q to stdout, want a rejected command line reported on stderr alone", out.String())
	}

	for _, fragment := range []string{"Error: ", "Usage:", "Examples:", "Flags:"} {
		if !strings.Contains(errOut.String(), fragment) {
			t.Errorf("execute() stderr =\n%s\nwant it to contain %q", errOut.String(), fragment)
		}
	}
}

// TestHelpPrintsTheCapturedDefault is the one case in which rendering the Flags
// block from the live Value diverges from rendering the table: the usage that
// follows a rejected command line would report the 2 that was typed as the
// default, and an operator would read it as the number a bare `stressy` starts.
func TestHelpPrintsTheCapturedDefault(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	var out bytes.Buffer
	cmd.stdout, cmd.stderr = &out, &out

	if err := cmd.execute([]string{"-w", "2", "4"}); err == nil {
		t.Fatal("execute() error = nil, want the argument to be rejected")
	}

	want := "(default " + strconv.Itoa(defaultWorkers()) + ")"
	if !strings.Contains(out.String(), want) {
		t.Errorf("execute() printed:\n%s\nwant the --workers line to end in %q", out.String(), want)
	}
}

// lookupSetting returns the table row a flag name is registered from.
func lookupSetting(c *command, long string) (setting, bool) {
	for _, s := range c.flags {
		if s.long == long {
			return s, true
		}
	}

	return setting{}, false
}
