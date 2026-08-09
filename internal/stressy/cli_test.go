package stressy

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// newTestCmd builds the real command with the stress test stubbed out and its
// output discarded.
func newTestCmd(t *testing.T, cfg *Cfg) *command {
	t.Helper()

	c := newCmd(cfg)
	c.run = func(*Cfg) error { return nil }
	c.stdout, c.stderr = io.Discard, io.Discard

	return c
}

// TestBoundedRunExecutesTheStressTest runs the real run, the join to the engine (#53).
func TestBoundedRunExecutesTheStressTest(t *testing.T) {
	const (
		timeout = 50 * time.Millisecond
		// As loose as internal/stressy's stopBudget, and for its reasons.
		budget = 30 * time.Second
	)

	cfg := Cfg{Out: io.Discard}
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
		{name: "workers", shorthand: "w", placeholder: "int", def: "1", wantUsage: []string{"parallel workers"}},
		// An int-of-seconds description gives no reason to try a duration (#26).
		{name: "timeout", shorthand: "t", placeholder: "duration", def: "0s", wantUsage: []string{"duration", "5m"}},
		{name: "report", shorthand: "r", placeholder: "duration", def: "0s", wantUsage: []string{"duration", "5m"}},
	}

	var cfg Cfg
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

// TestNothingIsReadFromTheEnvironment is the live guard on the removal: stressy
// took STRESSY_WORKERS, STRESSY_TIMEOUT and STRESSY_REPORT up to 0.5.0, and a
// compose file or pod spec still carrying them must run the flags it was given
// rather than the variables it was not.
func TestNothingIsReadFromTheEnvironment(t *testing.T) {
	for _, s := range []string{"WORKERS", "TIMEOUT", "REPORT", "HELP", "VERSION"} {
		t.Setenv("STRESSY_"+s, "8")
	}

	var cfg Cfg
	cmd := newTestCmd(t, &cfg)

	var ran bool
	cmd.run = func(*Cfg) error { ran = true; return nil }

	if err := cmd.execute([]string{"-w", "1", "-t", "100ms"}); err != nil {
		t.Fatalf("execute() error = %v, want the run the operator asked for", err)
	}

	if !ran {
		t.Error("run was not called, want the stress test to run")
	}

	if cfg.Workers != 1 || cfg.Timeout != 100*time.Millisecond || cfg.Report != 0 {
		t.Errorf("Workers, Timeout, Report = %d, %s, %s; want 1, 100ms, 0s", cfg.Workers, cfg.Timeout, cfg.Report)
	}
}

// TestBothSpellingsShareOneValue is what makes `-w 4`, `--workers 4`,
// `-workers 4` and `--w 4` one setting rather than two the flag package would
// otherwise keep apart.
func TestBothSpellingsShareOneValue(t *testing.T) {
	var cfg Cfg
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
		args        []string
		wantWorkers int
		wantTimeout time.Duration
		wantReport  time.Duration
	}{
		{
			name:        "flags",
			args:        []string{"-w", "4", "-t", "30s", "-r", "15s"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
			wantReport:  15 * time.Second,
		},
		{
			// Every spelling one Var pair admits, on one command line.
			name:        "every spelling of a flag",
			args:        []string{"--workers", "4", "-timeout", "30s", "--r", "15s"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
			wantReport:  15 * time.Second,
		},
		{
			name:        "only the flags given are set",
			args:        []string{"-w", "8"},
			wantWorkers: 8,
		},
		{
			// #104: the default was GOMAXPROCS(0), so a bare `stressy` started
			// a different number of workers on a laptop, in a container and in
			// a pod. One is the same number in all three.
			name:        "a bare stressy starts one worker",
			wantWorkers: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

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

// TestMalformedFlagValueIsRejected covers #50: strconv's internals reached the
// operator, and a value that named no valid spelling told them nothing.
func TestMalformedFlagValueIsRejected(t *testing.T) {
	tests := []struct {
		name  string
		flag  string
		other []string // a valid setting for the flag not under test
		value string
		want  string
	}{
		{name: "timeout", flag: "-t", other: []string{"-w", "1"}, value: "not-a-number", want: "want a duration such as 30s or 5m"},
		{name: "report", flag: "-r", other: []string{"-w", "1", "-t", "100ms"}, value: "not-a-number", want: "want a duration such as 30s or 5m"},
		// The bare-seconds spelling, accepted from 0.1.0 to 0.5.0 (#90's C).
		{name: "timeout as a bare number of seconds", flag: "-t", other: []string{"-w", "1"}, value: "60", want: "want a duration such as 30s or 5m"},
		{name: "report as a bare number of seconds", flag: "-r", other: []string{"-w", "1", "-t", "100ms"}, value: "60", want: "want a duration such as 30s or 5m"},
		{name: "workers", flag: "-w", other: []string{"-t", "100ms"}, value: "abc", want: "want a whole number"},
		{name: "workers, a float", flag: "-w", other: []string{"-t", "100ms"}, value: "2.0", want: "want a whole number"},
		{name: "workers, past what an int holds", flag: "-w", other: []string{"-t", "100ms"}, value: "99999999999999999999", want: "out of range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			err := cmd.execute(append([]string{tt.flag, tt.value}, tt.other...))
			if err == nil {
				t.Fatalf("execute(%s %s) error = nil, want a parse error", tt.flag, tt.value)
			}

			// Where the value came from, which one, and what a valid one is.
			for _, fragment := range []string{tt.flag, tt.value, tt.want} {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error = %q, want it to contain %q", err, fragment)
				}
			}

			if strings.Contains(err.Error(), "strconv") {
				t.Errorf("error = %q, want no strconv internals in it (#50)", err)
			}
		})
	}
}

// TestOutOfRangeValueIsRejected covers the range checks reached through the
// command: they run before the first worker starts, and they are not usage
// errors, so #17a keeps the flag list off them.
func TestOutOfRangeValueIsRejected(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// want is the whole message, not a fragment: the shape is the point.
		want string
	}{
		{name: "workers", args: []string{"-w", "0"}, want: "workers must be 1 or greater"},
		{name: "timeout", args: []string{"-w", "1", "-t", "-5s"}, want: "timeout must be 0 (indefinite) or greater"},
		{name: "report", args: []string{"-w", "1", "-r", "-30s"}, want: "report must be 0 (off) or greater"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			var ran bool
			cmd.run = func(*Cfg) error { ran = true; return nil }

			err := cmd.execute(tt.args)
			if err == nil {
				t.Fatalf("execute(%q) error = nil, want the value to be rejected", tt.args)
			}

			if err.Error() != tt.want {
				t.Errorf("execute(%q) error = %q, want %q", tt.args, err, tt.want)
			}

			if ran {
				t.Errorf("execute(%q) started the run, want the configuration rejected first", tt.args)
			}
		})
	}
}

// TestHelpAndVersionWorkOnTheCommandLine: both flags answer and run nothing.
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
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &out

			var ran bool
			cmd.run = func(*Cfg) error { ran = true; return nil }

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
			var cfg Cfg
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
			var cfg Cfg
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
		args        []string
		runErr      error
		wantSilence bool
	}{
		{name: "unknown flag", args: []string{"--bogus"}},
		{name: "invalid flag value", args: []string{"-t", "not-a-number"}},
		{name: "positional argument", args: []string{"4"}},
		// Why parseWorkers checks no range: the parser rejecting it means usage (#17a).
		{name: "out-of-range flag value", args: []string{"-w", "0"}, wantSilence: true},
		{name: "runtime failure", runErr: errors.New("workers must be 1 or greater"), wantSilence: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &out

			cmd.run = func(*Cfg) error { return tt.runErr }

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
	var cfg Cfg
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
	var cfg Cfg
	cmd := newTestCmd(t, &cfg)

	var out bytes.Buffer
	cmd.stdout, cmd.stderr = &out, &out

	if err := cmd.execute([]string{"-w", "2", "4"}); err == nil {
		t.Fatal("execute() error = nil, want the argument to be rejected")
	}

	// The literal, not the registered Value: reading the number back off the
	// code under test would leave this green whatever the two of them agreed on.
	want := "(default 1)"
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
