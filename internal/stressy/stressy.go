// Package stressy is the stressy command: the flag grammar it publishes and the
// CPU stress test that grammar configures, which worker goroutines run by
// hashing bcrypt. The root main.go is a call into Main and nothing else.
//
// What stressy publishes is that command line, not a Go API. The package is
// under internal/, so nothing here can be imported from outside this module,
// and 1.0.0 freezes the flags rather than any identifier below. Main is the
// whole of what main.go needs; a name exported past it is exported for a caller
// that cannot exist.
package stressy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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

// reportFloor is the shortest --report interval a run will start on. Below it a
// run spends itself formatting rather than hashing: `-t 1s -r 1ns` puts hundreds
// of thousands of lines and tens of megabytes on stdout for one second of work,
// which in a container with log shipping attached is most of the cost of the run
// and none of its purpose (#114).
const reportFloor = time.Second

// shutdownSignals are the signals that end a run. README.md's exit-code table
// documents what each one exits with; nothing holds the two together. Adding one
// means giving signalName a spelling for it, which a test does hold.
var shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// SignalError is what Run returns when a signal ended the run rather than the
// timer. It carries the signal because the exit code depends on which one fired.
// It is not a failure to report: Run has already printed the shutdown line and
// the run summary, so the caller turns this into an exit code rather than
// printing it again.
type SignalError struct {
	// Signal is the signal that triggered the shutdown, one of shutdownSignals.
	Signal os.Signal
}

// Error implements error. Nothing prints it on the normal path.
func (e *SignalError) Error() string {
	return "run interrupted by " + signalName(e.Signal)
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

	// Out is where a run prints its four kinds of line. nil is os.Stdout, which
	// is what the command leaves it as; a test reads a run through a buffer.
	Out io.Writer
}

// Run starts the configured workers and blocks until the timeout expires or a
// shutdown signal arrives, printing a progress line every report interval while
// it waits. It then waits for every worker to finish the hash it is on — the
// drain — and prints what the run did.
//
// That drain is one hash long only while the workers fit in GOMAXPROCS. Past it
// the hashes in flight finish in series, so the drain runs for roughly
// Workers/GOMAXPROCS of them: measured on 18 cores against `-t 1s`, `-w 18`
// ended about 0.2s past the deadline and `-w 2000` about 20s past it. Nothing
// caps Workers, because nothing here reads the machine (#104). What the length
// costs is said out loud instead — the shutdown line names what the wait is for,
// and signal handling is stopped before it, so a second signal kills the process
// rather than being buffered where nothing reads it again (#122).
//
// It returns an error if the configuration is invalid, a *SignalError — not a
// failure, an exit code — if a signal ended the run, and nil if the timer did.
func (c Cfg) Run() error {
	if err := c.validate(); err != nil {
		return err
	}

	// Defaulted before the first line is printed, and on the copy this value
	// receiver already holds, so waitForShutdown below prints to it too.
	if c.Out == nil {
		c.Out = os.Stdout
	}

	// Both shutdown triggers meet in one select — waitForShutdown's, below — over
	// one context, so two triggers cannot both fire. The buffer of 1 is what
	// makes a signal arriving before the select is reached a shutdown rather than
	// a lost one, and it is registered before the first line is printed: until it
	// is, either signal terminates the process outright and there is no shutdown
	// to report. TestExitCodes relies on that ordering.
	received := make(chan os.Signal, 1)
	signal.Notify(received, shutdownSignals...)

	// The guard for a Run that never reaches the stop below, which today means
	// a panic; the shutdown path does not wait for it, and cannot (#122).
	defer signal.Stop(received)

	writef(c.Out, "%s\n", c.startupMessage())

	if hint := c.hintMessage(); hint != "" {
		writef(c.Out, "%s\n", hint)
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

	// One shutdown is all this run has to report, and everything below it is the
	// drain. Handling stops here rather than at the deferred call, which does
	// not run until the drain is over: for the whole of a wait that grows with
	// Workers, a second signal landed in a one-deep buffer nobody read again and
	// the run could be stopped by nothing but SIGKILL (#122).
	//
	// Stopped rather than drained, so the signal goes back to the disposition it
	// had before Notify — the default, for anything a terminal, a `docker stop`
	// or a kubelet signals — and the second one ends the process where it
	// stands. On the timer path too, where the run was going to exit 0 and the
	// operator pressing Ctrl-C through the drain asked for something else.
	//
	// Before the line below rather than after it, so the log says the run is
	// draining only once a signal can in fact interrupt the drain.
	signal.Stop(received)

	writef(c.Out, "%s\n", shutdownMessage(sig))

	// Tells the workers to stop on the signal path; a no-op on the timer path,
	// where ctx is already done.
	stop()

	wg.Wait()

	writef(c.Out, "%s\n", c.summaryMessage(hashes.Load(), time.Since(start)))

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
			//
			// A signal arriving in the same instant leaves both cases ready, and
			// select picks between ready cases at random, so the deadline could
			// win a run a signal had ended: `Timer expired` on stdout and exit 0
			// where README.md's table says 143. Reading the channel first is what
			// makes that table true — a signal already in it ended this run (#117).
			//
			// A signal arriving after this returns is not caught at all: Run
			// stops handling the moment it has one shutdown to report, so the
			// second one ends the process outright rather than being buffered
			// where nothing reads it again. A run that had served its whole
			// timeout therefore dies with the signal instead of exiting 0,
			// which is what pressing Ctrl-C through the drain asks for (#122).
			select {
			case sig := <-received:
				return sig
			default:
				return nil
			}
		case <-tick:
			// time.Since rather than the timestamp the tick carries: a late tick
			// carries the time it fired, printing the elapsed time the line would
			// have had if the process were healthy — hiding exactly the pathology
			// an operator turns this on to see.
			writef(c.Out, "%s\n", progressMessage(hashes.Load(), time.Since(start)))
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

// drainNotice is the clause both shutdown lines end in: what the run is doing
// between that line and the summary under it.
//
// Without it a drain is a hang — nothing prints, and the length is not one hash
// but roughly Workers/GOMAXPROCS of them, which on a large `-w` is long enough
// for a pod's terminationGracePeriodSeconds to run out and SIGKILL the process
// before the summary or the exit code the README documents ever arrive. The log
// is the whole of what an operator has, the image being FROM scratch, so the
// wait says what it is for (#122).
//
// No count in it, deliberately: the startup line already says how many workers
// there are, and a line that changes shape with the configuration is one more
// thing for a script reading stdout to get wrong.
const drainNotice = "waiting for every worker to finish the hash it is on..."

// shutdownMessage says why the run is ending, and what it is waiting for before
// it does. sig is the signal that stopped it, or nil where the timer expired —
// the same distinction the exit code is chosen from further out.
//
// The signal is named rather than called "a signal", because otherwise the two
// signalled shutdowns print the same line while exiting 130 and 143, and telling
// those apart is what the exit-code table is for (#111).
func shutdownMessage(sig os.Signal) string {
	if sig == nil {
		return "Timer expired, shutting down; " + drainNotice
	}

	return fmt.Sprintf("Received %s, shutting down; %s", signalName(sig), drainNotice)
}

// signalName is what stressy calls a signal in the lines it prints: "SIGTERM",
// where os.Signal.String() would say "terminated". SIG* is the spelling
// README.md's exit-code table uses, so the name in a log and the code the table
// documents it under are one grep apart.
//
// Only shutdownSignals reach here from Run. Anything else falls back to String(),
// which is a worse name but still a name.
func signalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		return sig.String()
	}
}

// summaryMessage is the line Run prints once every worker has drained: what the
// run did, where the line above it says only why it stopped. The elapsed time is
// measured rather than the configured timeout echoed back, because a run ends
// past its deadline — by one hash where the workers fit in GOMAXPROCS and by
// roughly Workers/GOMAXPROCS of them where they do not — and the rate divides by
// the time that actually passed.
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

// validate reports whether a run can be started with this configuration.
// Timeout 0 is indefinite and Report 0 is off, and neither has an upper bound,
// because any length is one the operator asked for. A report interval has both
// bounds, because outside them it is not an interval anybody asked for: under
// reportFloor it is the run (#114), and past the timeout it is a line that never
// prints, which is the shape of `-r 1m` typed where `-r 1s` was meant (#115).
//
// Workers has no ceiling either. One far above the cores on offer is slow to
// shut down rather than wrong, and the number it would have to be measured
// against is GOMAXPROCS — which is the machine, and #104 settled that nothing
// here reads the machine. Run says what the wait is for instead (#122).
//
// dispatch calls it so a rejected configuration is reported before the first
// worker starts; Run calls it again for a caller that never came through the
// command. Both matter: Workers 0 starts no goroutines and idles forever, and
// Report -1 panics inside time.NewTicker.
func (c Cfg) validate() error {
	switch {
	case c.Workers < 1:
		return fmt.Errorf("workers must be 1 or greater")
	case c.Timeout < 0:
		return fmt.Errorf("timeout must be 0 (indefinite) or greater")
	case c.Report < 0, c.Report > 0 && c.Report < reportFloor:
		return fmt.Errorf("report must be 0 (off) or %s or greater", reportFloor)
	// An indefinite run outlives every interval, so only a bounded one can be
	// shorter than its own report. Equal is allowed and is the boundary: the
	// deadline is what ends that run, and whether the one tick landing with it
	// prints first is the runtime's to order.
	case c.Timeout > 0 && c.Report > c.Timeout:
		return fmt.Errorf("report %s is longer than timeout %s, so no progress line would print", c.Report, c.Timeout)
	}

	return nil
}

// stressTestCPU computes bcrypt hashes at hashCost until ctx is cancelled,
// counting each into hashes as it goes. That is where the whole of the load
// lives: bcrypt salts every call itself, so nothing outside the hash has to vary
// for the work to be real.
//
// A worker returns one hash after ctx is done — one hash, not one hash's worth
// of seconds. Past GOMAXPROCS workers that hash is sharing a core with the rest,
// so the wall clock it takes stretches with how many there are, and the run does
// not end until the last worker is through (#122).
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
