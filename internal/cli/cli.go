package cli

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// envPrefix prefixes the environment variables that configure stressy.
const envPrefix = "STRESSY"

// NewCmd builds the stressy command with its flags registered against cfg.
func NewCmd(cfg *stressy.Cfg) *cobra.Command {
	root := &cobra.Command{
		Use:   "stressy",
		Short: "Stressy is a simple tool to perform CPU stress tests",
		// Duplicated from the README because in the FROM scratch image --help is
		// the only documentation that ships.
		// TestPrecedenceIsDocumentedWhereUsersRead holds it to the flags bindEnv reads.
		Long: `Stressy is a simple tool to perform CPU stress tests.

The --workers, --timeout and --report flags can each be set from the environment
with the STRESSY_ prefix: STRESSY_WORKERS=4, STRESSY_TIMEOUT=5m or
STRESSY_REPORT=30s. A flag given on the command line beats its environment
variable, and an empty variable counts as unset.`,
		// TestDocumentedInvocationsAreValid runs every line of this through the
		// parser, so an example naming a flag that no longer exists fails the
		// build rather than the reader.
		Example: `  # Load the machine until interrupted
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
  docker run --rm --cpus 2 ghcr.io/felipeneuwald/stressy:latest -t 30s`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
		// Rejects operands, which cobra otherwise accepts and discards silently:
		// `stressy 4` would run the default count and say nothing about the 4. Not
		// cobra.NoArgs, which calls it an `unknown command` on a leaf command that
		// has no namespace for an argument to be an unknown member of.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q: stressy takes flags only", args[0])
			}

			return nil
		},
		// Flags not given on the command line are filled from the environment.
		PreRunE: func(c *cobra.Command, _ []string) error {
			// Set here rather than on the literal above, so an unknown or
			// unparseable flag still gets the flag list; everything that fails
			// from this point on is a runtime error, which does not want one.
			c.SilenceUsage = true

			fromEnv, err := bindEnv(c, envPrefix)
			if err != nil {
				return err
			}

			// The last point at which the source of a value is known. See
			// validateRanges.
			return validateRanges(cfg, fromEnv)
		},
		RunE: func(c *cobra.Command, _ []string) error {
			err := cfg.Run()

			// A signal-shortened run is reported by its exit code, not as an error:
			// Run has already printed the shutdown line, and cobra would report the
			// same shutdown twice. Silenced here alone; other errors are cobra's.
			if _, ok := errors.AsType[*stressy.SignalError](err); ok {
				c.SilenceErrors = true
			}

			return err
		},
	}

	// Var rather than IntVarP: the stock parser reports strconv's own error, which
	// names a Go standard library function at an operator. See workersValue.
	root.Flags().VarP(newWorkersValue(defaultWorkers(), &cfg.Workers), "workers", "w", "number of parallel workers for CPU stress testing; the default is the number of CPUs this process can use")

	// Var rather than DurationVarP: the stock parser rejects the bare-seconds
	// spelling this project has accepted since 0.1.0. See durationValue.
	root.Flags().VarP(newDurationValue(0, &cfg.Timeout), "timeout", "t", "how long to run the stress test, as a duration such as 30s or 5m; a bare number is seconds, and 0 runs until interrupted")

	// The same durationValue as --timeout, so `--report 60` reads as seconds too.
	// The usage text is ASCII, like every other string this program prints.
	root.Flags().VarP(newDurationValue(0, &cfg.Report), "report", "r", "how often to print a progress line carrying elapsed time, hashes and rate, as a duration such as 30s or 5m; a bare number is seconds, and 0, the default, prints none")

	return root
}

// defaultWorkers is how many workers a bare `stressy` starts: as many as this
// process can have running at once. GOMAXPROCS(0) rather than NumCPU, which
// ignores a cgroup CPU quota and so reports 64 on a 64-core node held to
// `limits.cpu: 2`. Passing 0 reads the value without setting it.
func defaultWorkers() int {
	return runtime.GOMAXPROCS(0)
}

// Main runs stressy and returns the code the process is to exit with. injected
// is the version stamped into release binaries, which the root main.go declares
// because .goreleaser.yaml stamps it as `main.injected`.
//
// The command is built here rather than at package level because cobra copies
// version into its Version field as it builds: a package-level command would be
// constructed before the line below runs, and every release binary would report
// the development placeholder instead of the version it was cut as.
func Main(injected string) int {
	version = resolveVersion(injected, buildInfo())

	err := NewCmd(&stressy.Cfg{}).Execute()
	if err == nil {
		return 0
	}

	// A run a signal cut short exits 128 plus the signal number, which is what
	// makes a Kubernetes Job record an evicted pod as failed rather than
	// Complete. RunE has already silenced this one, so nothing has printed it.
	if sig, ok := errors.AsType[*stressy.SignalError](err); ok {
		return sig.ExitCode()
	}

	// cobra has already printed the error.
	return 1
}
