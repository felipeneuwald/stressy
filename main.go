package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// envPrefix is the prefix on the environment variables that configure stressy:
// --workers is read from STRESSY_WORKERS, --timeout from STRESSY_TIMEOUT.
const envPrefix = "STRESSY"

var cmd = newCmd(&stressy.Cfg{})

// newCmd builds the stressy command with its flags registered against cfg. It
// is a constructor rather than a package-level literal so that tests can build
// an independent command over the same definitions.
func newCmd(cfg *stressy.Cfg) *cobra.Command {
	root := &cobra.Command{
		Use:   "stressy",
		Short: "Stressy is a simple tool to perform CPU stress tests",
		Long: `Stressy is a simple tool to perform CPU stress tests.

All flags can be configured using environment variables with the STRESSY_ prefix.
For example: STRESSY_WORKERS=4 or STRESSY_TIMEOUT=5m.`,
		// The README's usage block, carried into the binary. --help is where a
		// user looks before they look for a repository, and in the container
		// image it is the only documentation that ships: the image is FROM
		// scratch, so there is no README beside the binary and no shell to read
		// one with. It listed no examples at all, which left --help strictly
		// less informative than the README it paraphrased (#27c).
		//
		// TestDocumentedInvocationsAreValid runs every line of this through the
		// command, so an example naming a flag that no longer exists fails the
		// build rather than the reader.
		Example: `  # Load the machine until interrupted
  stressy

  # Four workers for five minutes
  stressy -w 4 -t 5m

  # A bare number is read as seconds, so pre-0.4 command lines keep working
  stressy -t 60

  # The same run, configured from the environment
  STRESSY_WORKERS=4 STRESSY_TIMEOUT=5m stressy

  # In a container the CPU limit sets the worker count
  docker run --rm --cpus 2 ghcr.io/felipeneuwald/stressy:latest -t 30s`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
		// stressy takes no arguments beyond its flags. Without this cobra
		// accepts and discards them, so `stressy 4` — a reasonable guess at
		// the worker count — ran the default number of workers and said
		// nothing about the 4 (#17b).
		//
		// Not cobra.NoArgs, which rejects them as `unknown command "4"`: this
		// command has no subcommands, so there is no command namespace for an
		// argument to be an unknown member of, and a user who typed a number
		// is left looking for a command they never meant to name. The usage
		// screen follows the error and lists the flags.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q: stressy takes flags only", args[0])
			}

			return nil
		},
		// Every flag not given on the command line is resolved from the
		// environment before the run starts; command-line flags win. See
		// bindEnv.
		PreRunE: func(c *cobra.Command, _ []string) error {
			// Set here rather than on the literal above, which would silence
			// usage on every error path including the two that are genuinely
			// usage errors: an unknown flag and an unparseable flag value both
			// want the flag list printed, and cobra has finished parsing flags
			// and validating operands by the time PreRunE runs. Anything that
			// fails from this point on is a runtime error, and a runtime error
			// followed by the whole help screen buries itself (#17a).
			c.SilenceUsage = true

			return bindEnv(c, envPrefix)
		},
		RunE: func(c *cobra.Command, _ []string) error {
			err := stressy.New(*cfg).Run()

			// A signal-shortened run is reported by its exit code, not as an
			// error: Run has already printed "Received signal, shutting
			// down..." and the run summary, and cobra printing `Error: run
			// interrupted by interrupt` under them would report one shutdown
			// twice, the second time as a failure (#48).
			//
			// Silenced here rather than on the command literal, for the same
			// reason SilenceUsage is set in PreRunE rather than there: every
			// other error — a rejected environment variable, an invalid worker
			// count — is still cobra's to print, and this is the one path where
			// the message has already been delivered.
			if _, ok := errors.AsType[*stressy.SignalError](err); ok {
				c.SilenceErrors = true
			}

			return err
		},
	}

	root.Flags().IntVarP(&cfg.Workers, "workers", "w", defaultWorkers(), "number of parallel workers for CPU stress testing; the default is the number of CPUs this process can use")

	// Var rather than DurationVarP: the stock pflag duration type would reject
	// the bare-seconds spelling this project has accepted since 0.1.0. See
	// durationValue.
	root.Flags().VarP(newDurationValue(0, &cfg.Timeout), "timeout", "t", "how long to run the stress test, as a duration such as 30s or 5m; a bare number is seconds, and 0 runs until interrupted")

	return root
}

// defaultWorkers is how many workers a bare `stressy` starts: as many as this
// process can have running at once. One worker loads about 6% of a 16-core
// machine, which is an odd thing for a CPU stress tool to do by default when
// the overwhelmingly common intent is to load the machine (#24).
//
// runtime.GOMAXPROCS(0) rather than runtime.NumCPU(): NumCPU reports the
// logical CPUs of the machine and knows nothing about a cgroup CPU quota, so
// on a 64-core node under `limits.cpu: 2` it returns 64 and the run spends
// itself being throttled instead of producing load — the containerised case
// this tool is most often deployed into, and the case a naive default makes
// worse rather than better. Since the go.mod language version reached 1.26 in
// 0.4.0 — past the 1.25 that gates the behaviour (#25) — the runtime's own
// default already accounts for the quota, the CPU affinity mask and the
// GOMAXPROCS environment variable, and what it yields is exactly the number of
// workers that can run simultaneously. Passing 0 reads
// that value without setting it, so the runtime keeps updating it if the quota
// changes.
func defaultWorkers() int {
	return runtime.GOMAXPROCS(0)
}

func main() {
	err := cmd.Execute()
	if err == nil {
		return
	}

	// A run a signal cut short exits 128 plus the signal number — 130 for
	// SIGINT, 143 for SIGTERM — where it used to exit 0, indistinguishable from
	// a run that served its whole `-t`. That distinction is the point: it is
	// what makes a Kubernetes Job record an evicted pod as failed rather than
	// Complete, and what gives the `backoffLimit: 0` in the README's manifest
	// something to bound (#48). RunE has already silenced this one, so nothing
	// has printed it.
	if sig, ok := errors.AsType[*stressy.SignalError](err); ok {
		os.Exit(sig.ExitCode())
	}

	// cobra has already printed the error.
	os.Exit(1)
}
