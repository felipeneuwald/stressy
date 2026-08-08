// Package stressy runs CPU stress tests with worker goroutines that hash bcrypt.
package stressy

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// hashCost is the bcrypt cost every worker hashes at. bcrypt doubles its work
// per increment, so the cost sets how long one uninterruptible
// GenerateFromPassword call runs, and so how long a worker takes to notice
// cancellation: ~0.18s per hash at cost 12, against ~26 hours at bcrypt.MaxCost,
// which pegs a core no harder and leaves the cancellation check unreachable.
const hashCost = 12

// shutdownSignals are the signals that end a run.
var shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// ShutdownSignals returns the signals a run stops on. Exported for
// TestDocumentedExitCodes, which holds the README's exit code table against it.
func ShutdownSignals() []os.Signal {
	return slices.Clone(shutdownSignals)
}

// SignalError is what Run returns when a signal ended the run rather than the
// timer. It carries the signal because the exit code depends on which one fired.
// It is not a failure to report: Run has already printed the shutdown line and
// the run summary, so the caller turns this into an exit code rather than
// printing it again.
type SignalError struct {
	// Signal is the signal that triggered the shutdown, one of ShutdownSignals.
	Signal os.Signal
}

// Error implements error. Nothing prints it on the normal path.
func (e *SignalError) Error() string {
	return "run interrupted by " + e.Signal.String()
}

// ExitCode is the status a run this signal ended should exit with: 128 plus the
// signal number, which is 130 for SIGINT and 143 for SIGTERM, the same code
// timeout(1) and the shells report. A signal that is not a syscall.Signal falls
// back to 1 rather than crashing a shutdown over an exit code.
func (e *SignalError) ExitCode() int {
	sig, ok := e.Signal.(syscall.Signal)
	if !ok {
		return 1
	}

	return 128 + int(sig)
}

// Cfg is a configured CPU stress test, ready to Run.
type Cfg struct {
	Workers int           // number of parallel worker goroutines
	Timeout time.Duration // how long to run (0 for indefinite)
	Report  time.Duration // how often to print a progress line (0 for never)
}

// Run starts the configured workers and blocks until the timeout expires or a
// shutdown signal arrives, printing a progress line every report interval while
// it waits. It then waits for every worker to finish the hash it is on, so a run
// ends up to one hash past its shutdown, and prints what the run did.
//
// It returns an error if the configuration is invalid, a *SignalError — not a
// failure, an exit code — if a signal ended the run, and nil if the timer did.
func (c Cfg) Run() error {
	if err := c.validateConfig(); err != nil {
		return err
	}

	// Both shutdown triggers meet in one select — waitForShutdown's, below — over
	// one context, so two triggers cannot both fire. The buffer of 1 is what
	// makes a signal arriving before the select is reached a shutdown rather than
	// a lost one, and it is registered before the first line is printed: until it
	// is, either signal terminates the process outright and there is no shutdown
	// to report. TestExitCodes relies on that ordering.
	received := make(chan os.Signal, 1)
	signal.Notify(received, shutdownSignals...)
	defer signal.Stop(received)

	fmt.Println(c.startupMessage())

	if hint := c.hintMessage(); hint != "" {
		fmt.Println(hint)
	}

	// Started before the deadline is set, so the elapsed time the summary
	// reports is never less than the timeout the operator asked for.
	start := time.Now()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	if c.Timeout > 0 {
		var expire context.CancelFunc
		ctx, expire = context.WithTimeout(ctx, c.Timeout)
		defer expire()
	}

	var wg sync.WaitGroup

	// Atomic because the --report heartbeat reads it mid-run, while every worker
	// is still adding to it.
	var hashes atomic.Uint64

	wg.Add(c.Workers)
	for range c.Workers {
		go func() {
			defer wg.Done()
			c.stressTestCPU(ctx, &hashes)
		}()
	}

	sig := c.waitForShutdown(ctx, received, &hashes, start)

	fmt.Println(shutdownMessage(sig))

	// Tells the workers to stop on the signal path; a no-op on the timer path,
	// where ctx is already done.
	stop()

	wg.Wait()

	fmt.Println(c.summaryMessage(hashes.Load(), time.Since(start)))

	if sig != nil {
		return &SignalError{Signal: sig}
	}

	return nil
}

// waitForShutdown blocks until the run ends, printing a progress line every
// report interval while it waits. It returns the signal that ended the run, or
// nil where the deadline expired — the distinction Run's shutdown line and the
// process exit code are both chosen from.
func (c Cfg) waitForShutdown(ctx context.Context, received <-chan os.Signal, hashes *atomic.Uint64, start time.Time) os.Signal {
	// nil where --report is off, and a receive from a nil channel blocks forever,
	// so the default run waits on exactly the two channels it always did.
	var tick <-chan time.Time

	if c.Report > 0 {
		ticker := time.NewTicker(c.Report)
		defer ticker.Stop()

		tick = ticker.C
	}

	for {
		select {
		case sig := <-received:
			return sig
		case <-ctx.Done():
			// Both of Run's cancel functions are deferred, so this branch means
			// the deadline expired; nil is what tells the two shutdowns apart.
			return nil
		case <-tick:
			// time.Since rather than the timestamp the tick carries: a late tick
			// carries the time it fired, printing the elapsed time the line would
			// have had if the process were healthy — hiding exactly the pathology
			// an operator turns this on to see.
			fmt.Println(progressMessage(hashes.Load(), time.Since(start)))
		}
	}
}

// startupMessage is the line Run prints before the workers start: how many
// workers, and for how long. Built as a string rather than printed in place,
// like the four message functions below, so it is testable without os.Stdout.
func (c Cfg) startupMessage() string {
	// The timeout is a time.Duration and formats itself: "30s", "5m0s".
	duration := "indefinitely"
	if c.Timeout > 0 {
		duration = "for " + c.Timeout.String()
	}

	return fmt.Sprintf("Starting CPU stress test with %d %s %s", c.Workers, plural(c.Workers, "worker", "workers"), duration)
}

// hintMessage is the second line Run prints, and only on an indefinite run —
// the one that has to say how to stop it. SIGTERM is named beside Ctrl-C
// because that is what a `docker stop` or a node drain sends.
func (c Cfg) hintMessage() string {
	if c.Timeout > 0 {
		return ""
	}

	return "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information"
}

// progressMessage is the line a --report run prints on every tick. Its rate is
// cumulative rather than per-interval, so the last progress line of a run and
// the summary under it agree.
func progressMessage(hashes uint64, elapsed time.Duration) string {
	return fmt.Sprintf(
		"%s elapsed, %d %s, %.1f hashes/s",
		elapsed.Round(time.Millisecond),
		hashes, plural(hashes, "hash", "hashes"),
		hashRate(hashes, elapsed),
	)
}

// shutdownMessage says why the run is ending. sig is the signal that stopped it,
// or nil where the timer expired — the same distinction the exit code is chosen
// from further out.
func shutdownMessage(sig os.Signal) string {
	if sig == nil {
		return "Timer expired, shutting down..."
	}

	return "Received signal, shutting down..."
}

// summaryMessage is the line Run prints once every worker has drained: what the
// run did, where the line above it says only why it stopped. The elapsed time is
// measured rather than the configured timeout echoed back, because a run ends up
// to one hash past its deadline and the rate divides by the time that passed.
func (c Cfg) summaryMessage(hashes uint64, elapsed time.Duration) string {
	return fmt.Sprintf(
		"Computed %d %s in %s (%.1f hashes/s, %d %s)",
		hashes, plural(hashes, "hash", "hashes"),
		// Rounded: the digits below a millisecond are noise against a hash that
		// costs two hundred of them.
		elapsed.Round(time.Millisecond),
		hashRate(hashes, elapsed),
		c.Workers, plural(c.Workers, "worker", "workers"),
	)
}

// hashRate is the rate both reporting lines quote. The guard is why it is worth
// a function: dividing by a zero elapsed time would print "+Inf hashes/s".
func hashRate(hashes uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}

	return float64(hashes) / elapsed.Seconds()
}

// plural picks the form of a noun that goes with n.
func plural[T int | uint64](n T, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// ValidateWorkers reports whether workers is a count a run can be started with.
// Exported, like the other validators here, because the command layer checks it
// before the run — where it still knows whether the value came from `-w` or from
// STRESSY_WORKERS and can name the one that produced it. This package cannot.
func ValidateWorkers(workers int) error {
	if workers < 1 {
		return fmt.Errorf("workers must be 1 or greater")
	}

	return nil
}

// ValidateTimeout reports whether timeout can start a run; 0 means indefinite.
func ValidateTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("timeout must be 0 (indefinite) or greater")
	}

	return nil
}

// ValidateReport reports whether report can start a run; 0, the default, means
// off. No upper bound on purpose: any interval is one the operator asked for.
func ValidateReport(report time.Duration) error {
	if report < 0 {
		return fmt.Errorf("report must be 0 (off) or greater")
	}

	return nil
}

// validateConfig applies the three rules above before a single worker starts.
// The command checks the same settings first, so this is the backstop for every
// other caller of Run.
func (c Cfg) validateConfig() error {
	if err := ValidateWorkers(c.Workers); err != nil {
		return err
	}

	if err := ValidateTimeout(c.Timeout); err != nil {
		return err
	}

	return ValidateReport(c.Report)
}

// stressTestCPU computes bcrypt hashes at hashCost until ctx is cancelled,
// counting each into hashes as it goes. That is where the whole of the load
// lives: bcrypt salts every call itself, so nothing outside the hash has to vary
// for the work to be real, and a worker returns within one hash of ctx being
// done.
func (c Cfg) stressTestCPU(ctx context.Context, hashes *atomic.Uint64) {
	// Hoisted; a constant also stays well inside bcrypt's 72-byte limit.
	password := []byte("stressy")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Unreachable in practice: the cost is a valid constant and the
			// password is seven bytes, which leaves salt generation as the
			// only error source, and crypto/rand no longer reports failure.
			if _, err := bcrypt.GenerateFromPassword(password, hashCost); err != nil {
				panic(err)
			}

			hashes.Add(1)
		}
	}
}
