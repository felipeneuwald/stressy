package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// envPrefix prefixes the environment variables that configure stressy.
const envPrefix = "STRESSY"

// name is what stressy calls itself in the lines it prints about itself.
const name = "stressy"

// useLine is the one line under `Usage:`. stressy has no subcommands and takes
// no operands, so the whole of its grammar is its flags.
const useLine = "  " + name + " [flags]"

// Description is what `stressy --help` prints above the usage block.
// Duplicated from the README because in the FROM scratch image --help is the
// only documentation that ships.
// TestPrecedenceIsDocumentedWhereUsersRead holds it to the settings bindEnv fills.
const Description = `Stressy is a simple tool to perform CPU stress tests.

The --workers, --timeout and --report flags can each be set from the environment
with the STRESSY_ prefix: STRESSY_WORKERS=4, STRESSY_TIMEOUT=5m or
STRESSY_REPORT=30s. A flag given on the command line beats its environment
variable, and an empty variable counts as unset.`

// Examples is the block `stressy --help` prints under `Examples:`.
// TestDocumentedInvocationsAreValid runs every line of this through the parser,
// so an example naming a flag that no longer exists fails the build rather than
// the reader.
const Examples = `  # Load the machine until interrupted
  stressy

  # Four workers for five minutes
  stressy -w 4 -t 5m

  # A bare number is read as seconds, so pre-0.4 command lines keep working
  stressy -t 60

  # The same run, configured from the environment
  STRESSY_WORKERS=4 STRESSY_TIMEOUT=5m stressy

  # A progress line every 30 seconds; without one a run prints nothing until it ends
  stressy -t 30m -r 30s

  # In a container the CPU limit sets the worker count
  docker run --rm --cpus 2 ghcr.io/felipeneuwald/stressy:latest -t 30s`

// setting is one row of the flag table: how a flag registers, how the Flags
// block prints it, and whether the environment may fill it. One table drives
// registration and rendering both, which is what leaves `--help` unable to
// disagree with the flags the binary actually has.
type setting struct {
	long        string
	short       string
	placeholder string // the type printed after the name; empty for a bool
	usage       string
	def         string // the default as of registration; empty where none prints
	env         bool   // whether the environment may fill this setting
	value       flag.Value
}

// Setting is one row of the flag table, as the documentation tests read it.
type Setting struct {
	Long, Short, Usage string
	EnvVar             string // "" when the environment cannot fill this flag
}

// Settings returns the flag table. EnvVar is empty for --help and --version.
//
// This, and never the FlagSet, is what a test walking stressy's settings
// iterates: every setting is registered twice, once under each spelling, so a
// loop over the set sees `w` a second time and calls the shorthand a setting of
// its own that nothing documents.
func Settings() []Setting {
	flags := newCmd(&stressy.Cfg{}).flags

	settings := make([]Setting, 0, len(flags))
	for _, s := range flags {
		row := Setting{Long: s.long, Short: s.short, Usage: s.usage}
		if s.env {
			row.EnvVar = envName(envPrefix, s.long)
		}

		settings = append(settings, row)
	}

	return settings
}

// usageError is a command line the parser rejected: an unknown flag, a value it
// could not read, an argument where only flags go. Everything past parsing —
// a variable out of range, a run that failed — is not one.
//
// The split of #17a is carried by this type rather than by a flag flipped
// partway through the run, so what earns the flag list is a property of the
// error itself and cannot be decided at the wrong moment.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

// command is a stressy command line: the flag table, the set both spellings of
// every flag are registered in, and the seam a test replaces.
type command struct {
	cfg   *stressy.Cfg
	fs    *flag.FlagSet
	flags []setting

	wantHelp    bool
	wantVersion bool

	// run is the stress test itself, replaced by a test that is about what a
	// command line configures rather than about pegging a CPU for the length of
	// one. stdout and stderr are the same seam for what the command prints.
	run    func(*stressy.Cfg) error
	stdout io.Writer
	stderr io.Writer
}

// newCmd builds the stressy command with its flags registered against cfg.
func newCmd(cfg *stressy.Cfg) *command {
	c := &command{
		cfg:    cfg,
		fs:     flag.NewFlagSet(name, flag.ContinueOnError),
		run:    func(cfg *stressy.Cfg) error { return cfg.Run() },
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	// The flag package prints its own error and calls Usage before handing the
	// error back. Both are silenced here, because execute prints the error and
	// the block that goes with it from the table below.
	c.fs.SetOutput(io.Discard)
	c.fs.Usage = func() {}

	// Var rather than IntVar: the stock parser reports strconv's own error,
	// which names a Go standard library function at an operator. See workersValue.
	workers := newWorkersValue(defaultWorkers(), &cfg.Workers)

	// Var rather than DurationVar: the stock parser rejects the bare-seconds
	// spelling this project has accepted since 0.1.0. See durationValue.
	timeout := newDurationValue(0, &cfg.Timeout)

	// The same durationValue as --timeout, so `--report 60` reads as seconds too.
	// The usage text is ASCII, like every other string this program prints.
	report := newDurationValue(0, &cfg.Report)

	// Alphabetical, which is the order the Flags block prints them in; nothing
	// sorts this at render time, so `sort` stays out of the build graph.
	//
	// Each default is captured here rather than read back off the Value, which
	// by then may hold what a rejected command line left in it: `stressy -w 2 4`
	// prints its flag list after the error, and the --workers line there has to
	// say what a bare `stressy` starts rather than echoing the 2 that was typed.
	c.flags = []setting{
		{
			long: "help", short: "h", usage: "help for " + name,
			value: newBoolValue(&c.wantHelp),
		},
		{
			long: "report", short: "r", placeholder: report.Type(), def: report.String(), env: true,
			usage: "how often to print a progress line carrying elapsed time, hashes and rate, as a duration such as 30s or 5m; a bare number is seconds, and 0, the default, prints none",
			value: report,
		},
		{
			long: "timeout", short: "t", placeholder: timeout.Type(), def: timeout.String(), env: true,
			usage: "how long to run the stress test, as a duration such as 30s or 5m; a bare number is seconds, and 0 runs until interrupted",
			value: timeout,
		},
		{
			long: "version", short: "v", usage: "version for " + name,
			value: newBoolValue(&c.wantVersion),
		},
		{
			long: "workers", short: "w", placeholder: workers.Type(), def: workers.String(), env: true,
			usage: "number of parallel workers for CPU stress testing; the default is the number of CPUs this process can use",
			value: workers,
		},
	}

	// Twice over one Value, because the flag package knows no shorthands and
	// draws no distinction between one dash and two: this is what makes `-w 4`,
	// `--workers 4`, `-workers 4` and `--w 4` all reach the same setting.
	for _, s := range c.flags {
		c.fs.Var(s.value, s.long, s.usage)
		c.fs.Var(s.value, s.short, s.usage)
	}

	return c
}

// execute runs the command line and returns what went wrong, having already
// printed it: an error on its own, and for a usage error the flag list under
// it. The error comes back as well because the exit code is chosen from it.
func (c *command) execute(args []string) error {
	err := c.dispatch(args)
	if err == nil {
		return nil
	}

	// A signal-shortened run is reported by its exit code, not as an error: Run
	// has already printed the shutdown line, and printing this would report the
	// same shutdown twice.
	if _, ok := errors.AsType[*stressy.SignalError](err); ok {
		return err
	}

	writef(c.stderr, "Error: %v\n", err)

	// A mistyped flag wants the flag list; everything from bindEnv on is a
	// rejected value or a failed run, which wants one line (#17a).
	if _, ok := errors.AsType[*usageError](err); ok {
		writef(c.stderr, "%s\n", c.usage())
	}

	return err
}

// dispatch parses args and does what they ask for, printing nothing but the
// help screen and the version — the two answers that are not errors.
func (c *command) dispatch(args []string) error {
	if err := c.fs.Parse(args); err != nil {
		return &usageError{err}
	}

	if c.wantHelp {
		writef(c.stdout, "%s\n\n%s", Description, c.usage())

		return nil
	}

	if c.wantVersion {
		writef(c.stdout, "%s version %s\n", name, version)

		return nil
	}

	// Rejects operands rather than discarding them silently: `stressy 4` would
	// otherwise run the default count and say nothing about the 4 (#17b).
	if rest := c.fs.Args(); len(rest) > 0 {
		return &usageError{fmt.Errorf("unexpected argument %q: stressy takes flags only", rest[0])}
	}

	// Flags not given on the command line are filled from the environment.
	fromEnv, err := bindEnv(c.fs, c.flags, envPrefix)
	if err != nil {
		return err
	}

	// The last point at which the source of a value is known. See validateRanges.
	if err := validateRanges(c.cfg, fromEnv); err != nil {
		return err
	}

	return c.run(c.cfg)
}

// writef prints to one of the command's two streams and drops the error a
// closed stdout would give back: there is nowhere left to report that with, and
// no exit code this program has would be truer for it.
func writef(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// usage is the block under `--help` and under an error the parser produced: how
// stressy is invoked, the examples, and the flags. It ends in a newline, so the
// error path adds one of its own and leaves a blank line at the bottom.
func (c *command) usage() string {
	var b strings.Builder

	b.WriteString("Usage:\n")
	b.WriteString(useLine)
	b.WriteString("\n\nExamples:\n")
	b.WriteString(Examples)
	b.WriteString("\n\nFlags:\n")

	c.writeFlags(&b)

	return b.String()
}

// writeFlags renders one line per setting, every usage text starting at the
// same column: three past the longest `  -x, --name PLACEHOLDER` prefix. No
// wrapping, so the long --report line is emitted whole however narrow the
// terminal is, and no sorting, because the table is already in help order.
func (c *command) writeFlags(b *strings.Builder) {
	prefixes := make([]string, len(c.flags))

	var width int

	for i, s := range c.flags {
		prefixes[i] = "  -" + s.short + ", --" + s.long
		if s.placeholder != "" {
			prefixes[i] += " " + s.placeholder
		}

		width = max(width, len(prefixes[i])+3)
	}

	for i, s := range c.flags {
		b.WriteString(prefixes[i])
		b.WriteString(strings.Repeat(" ", width-len(prefixes[i])))
		b.WriteString(s.usage)

		// --help and --version have no default worth printing; the other three
		// print theirs even when it is the zero value, `(default 0s)` included.
		if s.def != "" {
			b.WriteString(" (default ")
			b.WriteString(s.def)
			b.WriteString(")")
		}

		b.WriteByte('\n')
	}
}

// defaultWorkers is how many workers a bare `stressy` starts: as many as this
// process can have running at once. GOMAXPROCS(0) rather than NumCPU, which
// ignores a cgroup CPU quota and so reports 64 on a 64-core node held to
// `limits.cpu: 2`. Passing 0 reads the value without setting it.
func defaultWorkers() int {
	return runtime.GOMAXPROCS(0)
}

// Parse resolves one command line into cfg the way a run does — flags, then the
// environment, then the range checks — and starts no stress test. That is the
// seam the documentation tests drive: they are about what a published
// invocation configures, not about pegging a CPU for the length of one.
func Parse(cfg *stressy.Cfg, args []string) error {
	c := newCmd(cfg)
	c.run = func(*stressy.Cfg) error { return nil }
	c.stdout, c.stderr = io.Discard, io.Discard

	return c.execute(args)
}

// Main runs stressy and returns the code the process is to exit with. injected
// is the version stamped into release binaries, which the root main.go declares
// because .goreleaser.yaml stamps it as `main.injected`.
func Main(injected string) int {
	version = resolveVersion(injected, buildInfo())

	err := newCmd(&stressy.Cfg{}).execute(os.Args[1:])
	if err == nil {
		return 0
	}

	// A run a signal cut short exits 128 plus the signal number, which is what
	// makes a Kubernetes Job record an evicted pod as failed rather than
	// Complete. execute has already silenced this one, so nothing printed it.
	if sig, ok := errors.AsType[*stressy.SignalError](err); ok {
		return sig.ExitCode()
	}

	// execute has already printed the error.
	return 1
}
