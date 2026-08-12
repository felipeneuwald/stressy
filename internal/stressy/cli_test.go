package stressy

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// newTestCmd builds the real command with the stress test stubbed out and its
// output discarded. The empty injected version is every build path but a
// release, so what --version reports here is what resolveVersion falls back to.
func newTestCmd(t *testing.T, cfg *Cfg) *command {
	t.Helper()

	c := newCmd(cfg, "")
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

	// One seam for both: the command's stdout is what the run prints through.
	var cfg Cfg

	cmd := newCmd(&cfg, "")
	cmd.stdout, cmd.stderr = io.Discard, io.Discard

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
		// Both bounds are stated where a command line is typed from (#114, #115).
		{name: "report", shorthand: "r", placeholder: "duration", def: "0s", wantUsage: []string{"duration", "5m", "no shorter than 1s", "no longer than --timeout"}},
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

			// Once each, which is #123: the flag package already wraps what Set
			// returns in `invalid value %q for flag -%s:`, so a Set naming the
			// value itself made one line say it twice.
			if got := strings.Count(err.Error(), tt.value); got != 1 {
				t.Errorf("error = %q names %q %d times, want once (#123)", err, tt.value, got)
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
		{name: "report", args: []string{"-w", "1", "-r", "-30s"}, want: "report must be 0 (off) or 1s or greater"},
		// #114: a run that formats rather than hashes is not one anybody asked for.
		{name: "report under the floor", args: []string{"-w", "1", "-t", "1s", "-r", "1ns"}, want: "report must be 0 (off) or 1s or greater"},
		// #115: a run whose ticker never fires, which is `-r 1s` mistyped.
		{name: "report longer than the timeout", args: []string{"-w", "1", "-t", "3s", "-r", "1m"}, want: "report 1m0s is longer than timeout 3s, so no progress line would print"},
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
	// What this binary resolves for itself, which `go test` stamps nothing into.
	version := newCmd(&Cfg{}, "").version

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
//
// The last two cases are #124. Both flags answered above the check and returned
// before it, so `stressy -h 4` printed the help screen and exited 0 with the 4
// discarded — the same silent discard #17b was about, and one that left
// README.md's exit-code table true only of a line carrying neither flag.
func TestPositionalArgumentsAreRejected(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantArg string
		// wantUnprinted is the answer a flag on the line would have given, and
		// which the rejected command line must not have produced.
		wantUnprinted string
	}{
		{name: "bare argument", args: []string{"4"}, wantArg: "4"},
		{name: "several arguments", args: []string{"foo", "bar", "4"}, wantArg: "foo"},
		{name: "argument after a flag", args: []string{"-w", "4", "extra"}, wantArg: "extra"},
		// The help screen opens with the description; the flag list under the
		// error does not, so this names the branch that must not have been taken.
		{name: "argument after --help", args: []string{"-h", "4"}, wantArg: "4", wantUnprinted: description},
		{name: "arguments after --version", args: []string{"-v", "extra", "junk"}, wantArg: "extra", wantUnprinted: "stressy version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &out

			var ran bool
			cmd.run = func(*Cfg) error { ran = true; return nil }

			err := cmd.execute(tt.args)
			if err == nil {
				t.Fatalf("execute(%q) error = nil, want the argument to be rejected", tt.args)
			}

			if !strings.Contains(err.Error(), tt.wantArg) {
				t.Errorf("execute(%q) error = %q, want it to name %q", tt.args, err, tt.wantArg)
			}

			if ran {
				t.Errorf("execute(%q) started the run, want the command line rejected first", tt.args)
			}

			if tt.wantUnprinted != "" && strings.Contains(out.String(), tt.wantUnprinted) {
				t.Errorf("execute(%q) printed:\n%s\nwant no %q in it: an unreadable command line is answered by the error, not by the flag on it (#124)", tt.args, out.String(), tt.wantUnprinted)
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
			var usageErr *usageError
			if isUsage := errors.As(err, &usageErr); isUsage == tt.wantSilence {
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

	for _, fragment := range []string{"Error: ", "Usage:", "Flags:"} {
		if !strings.Contains(errOut.String(), fragment) {
			t.Errorf("execute() stderr =\n%s\nwant it to contain %q", errOut.String(), fragment)
		}
	}
}

// TestOnlyHelpPrintsTheExamples is the second half of #116: a usage error used
// to print all four examples above the flag table — 24 lines for one wrong
// character, and more than that now the table wraps. The table stays,
// because it is the answer to what the flag could have been; the examples are
// for a first reading of `--help`, which still has them.
func TestOnlyHelpPrintsTheExamples(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantExamples bool
	}{
		{name: "help", args: []string{"--help"}, wantExamples: true},
		{name: "an unknown flag", args: []string{"--bogus"}},
		{name: "a value the parser rejects", args: []string{"-t", "not-a-number"}},
		{name: "a positional argument", args: []string{"4"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Cfg
			cmd := newTestCmd(t, &cfg)

			var out bytes.Buffer
			cmd.stdout, cmd.stderr = &out, &out

			_ = cmd.execute(tt.args)

			if got := strings.Contains(out.String(), "Examples:"); got != tt.wantExamples {
				t.Errorf("execute(%q) printed:\n%s\nwant the examples block printed = %t (#116)", tt.args, out.String(), tt.wantExamples)
			}

			// Whichever block it was, the flags are in it: that is what names
			// the flag a mistyped one could have been.
			if !strings.Contains(out.String(), "Flags:") {
				t.Errorf("execute(%q) printed:\n%s\nwant the flag table in it", tt.args, out.String())
			}
		})
	}
}

// TestHelpFitsEightyColumns is the first half of #116: three usage texts ran to
// 175, 165 and 144 columns, each of which an 80-column terminal wrapped twice at
// an arbitrary point and with no indent under it. 80 is hard-coded, because the
// alternative is golang.org/x/term for the width and this program has one direct
// dependency.
func TestHelpFitsEightyColumns(t *testing.T) {
	// The literal, not helpWidth: reading the width back off the code under test
	// would leave this green at whatever width the two of them agreed on, and 80
	// is the decision 1.0.0 freezes rather than an implementation detail.
	const columns = 80

	var cfg Cfg
	cmd := newTestCmd(t, &cfg)

	screens := map[string]string{
		"--help":        description + "\n\n" + cmd.usage(withExamples),
		"a usage error": cmd.usage(withoutExamples),
	}

	for name, screen := range screens {
		t.Run(name, func(t *testing.T) {
			for line := range strings.SplitSeq(strings.TrimSuffix(screen, "\n"), "\n") {
				// Runes, not bytes: a column is what a terminal wraps on. The
				// two are the same number here only because every string this
				// program prints about itself is ASCII.
				if n := utf8.RuneCountInString(line); n > columns {
					t.Errorf("%s printed a %d-column line, want %d or fewer:\n%s", name, n, columns, line)
				}
			}
		})
	}
}

// TestFlagDescriptionsWrapUnderTheirColumn holds the shape of a wrapped row: a
// description that runs past the margin continues under itself rather than under
// the flag, which is what the terminal's own wrapping could not do.
func TestFlagDescriptionsWrapUnderTheirColumn(t *testing.T) {
	var cfg Cfg
	cmd := newTestCmd(t, &cfg)

	var b strings.Builder

	cmd.writeFlags(&b)

	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")

	// The column the first row's description starts at, read off the render
	// rather than recomputed from the table, which would only restate the code.
	column := strings.Index(lines[0], "help for")
	if column < 0 {
		t.Fatalf("the flag table opens with %q, want the --help row", lines[0])
	}

	var wrapped int

	for _, line := range lines {
		// A row of its own starts with the two-space indent and a shorthand.
		if strings.HasPrefix(line, "  -") {
			continue
		}

		wrapped++

		if got := len(line) - len(strings.TrimLeft(line, " ")); got != column {
			t.Errorf("a continuation line starts at column %d, want the description column %d:\n%s", got, column, line)
		}
	}

	// A table nothing wraps would leave the assertion above running on nothing.
	if wrapped == 0 {
		t.Errorf("the flag table wrapped no line, so this test asserts nothing:\n%s", b.String())
	}
}

// TestWrapText covers the breaking itself, which the table above exercises only
// at the widths its own strings happen to have.
func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{name: "a text shorter than the width", text: "one two", width: 20, want: []string{"one two"}},
		{name: "a text broken on its spaces", text: "one two three four", width: 9, want: []string{"one two", "three", "four"}},
		{name: "a line filled exactly", text: "one two three", width: 7, want: []string{"one two", "three"}},
		// A flag spelling or a ghcr.io reference is worth more whole than split.
		{name: "a word longer than the width", text: "a --report=1000000000ns b", width: 5, want: []string{"a", "--report=1000000000ns", "b"}},
		{name: "no text at all", text: "", width: 10, want: []string{""}},
		{name: "a width nothing fits in", text: "one two", width: 0, want: []string{"one two"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.width)

			if !slices.Equal(got, tt.want) {
				t.Errorf("wrapText(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
			}
		})
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
