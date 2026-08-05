// Package stressy provides CPU stress testing functionality through controlled
// worker goroutines that perform intensive cryptographic operations.
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

// hashCost is the bcrypt cost every worker hashes at.
//
// bcrypt doubles its work per increment, so the cost is not a load setting —
// it is how long one uninterruptible GenerateFromPassword call runs, and
// therefore how long a worker takes to notice cancellation, since it can only
// check between calls. Cost 12 measures ~0.18s per hash on an M-series core.
// bcrypt.MaxCost, which this used to use, measures ~26 hours by the same
// doubling: it pegged a core no harder and made the cancellation check below
// unreachable (#15).
const hashCost = 12

// shutdownSignals are the signals that end a run. Both are ordinary requests to
// stop — Ctrl-C at a terminal, a `docker stop`, a kubelet draining a node — and
// each is reported by the exit code SignalError carries.
var shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// ShutdownSignals returns the signals a run stops on.
//
// It is exported for TestDocumentedExitCodes, which holds the README's exit
// code table against this list: a signal handled here and documented nowhere is
// a status an operator's supervisor reports and cannot look up, which is the
// same class of silent gap as the release artefact nobody missed in #28.
func ShutdownSignals() []os.Signal {
	return slices.Clone(shutdownSignals)
}

// SignalError is what Run returns when a signal ended the run rather than the
// timer. It carries the signal because the exit code depends on which one fired.
//
// It exists because the two shutdowns used to be indistinguishable from outside
// the process: a pod SIGTERM'd five seconds into a `-t 60s` run exited 0,
// exactly like one that ran the full minute, so the Kubernetes Job around it
// recorded an evicted run as Complete and the `backoffLimit: 0` guarding it
// never saw the failure it exists to bound (#48).
//
// It is not a failure to report. Run has already printed "Received signal,
// shutting down..." and the run summary by the time it returns this, so the
// caller's job is to turn it into an exit code, not to print it a second time.
type SignalError struct {
	// Signal is the signal that triggered the shutdown, one of ShutdownSignals.
	Signal os.Signal
}

// Error implements error. Nothing prints it on the normal path — see the note
// on double reporting above — so it is written for a caller that logs an
// unexpected error rather than for an operator reading a terminal.
func (e *SignalError) Error() string {
	return "run interrupted by " + e.Signal.String()
}

// ExitCode is the status a run this signal ended should exit with: 128 plus the
// signal number, which is 130 for SIGINT and 143 for SIGTERM. That is what
// timeout(1), the shells and every process killed by an unhandled signal
// already report, so a supervisor reading exit codes needs to know nothing
// about stressy to read it.
//
// A signal that is not a syscall.Signal falls back to 1 rather than panicking:
// every value in shutdownSignals is one on every platform releases build for,
// and an exit code is not worth crashing a shutdown over if that stops being
// true.
func (e *SignalError) ExitCode() int {
	sig, ok := e.Signal.(syscall.Signal)
	if !ok {
		return 1
	}

	return 128 + int(sig)
}

// Stressy represents a CPU stress testing instance.
// It manages multiple worker goroutines that perform CPU-intensive operations
// and handles graceful shutdown through signals or timeouts.
type Stressy struct {
	workers int           // number of parallel worker goroutines
	timeout time.Duration // how long to run (0 for indefinite)
	report  time.Duration // how often to print a progress line (0 for never)
}

// Cfg holds the configuration parameters for creating a new Stressy instance.
type Cfg struct {
	Workers int           // number of parallel worker goroutines
	Timeout time.Duration // how long to run (0 for indefinite)
	Report  time.Duration // how often to print a progress line (0 for never)
}

// New creates and returns a new Stressy instance with the given configuration.
// The instance is ready to start but not yet running.
func New(c Cfg) *Stressy {
	return &Stressy{
		workers: c.Workers,
		timeout: c.Timeout,
		report:  c.Report,
	}
}

// Run starts the CPU stress test with the configured number of workers.
// It handles:
//   - Configuration validation
//   - Worker goroutine management
//   - Timer management (if timeout > 0)
//   - Signal handling (SIGINT, SIGTERM)
//   - Graceful shutdown
//
// The test runs until either:
//   - The timeout duration is reached (if configured)
//   - An interrupt signal is received
//
// While it waits, a run given a report interval prints a progress line on every
// tick. Without one it prints nothing at all between the startup lines and the
// shutdown line, however long the run (#70).
//
// Run then waits for every worker to finish the hash it is on, so it returns
// up to one hash after the shutdown was triggered, and prints what the run did
// once they have all drained.
//
// It returns an error if the configuration is invalid, and a *SignalError — not
// a failure, an exit code — if a signal ended the run rather than the timer.
// A run that completes its timeout returns nil.
func (s *Stressy) Run() error {
	if err := s.validateConfig(); err != nil {
		return err
	}

	// Both shutdown triggers meet in one select — waitForShutdown's, below —
	// over one context that only this function cancels. That is the shape #14
	// left behind: two hand-rolled `close(s.done)` calls, one per trigger, with
	// nothing coordinating them, so a signal arriving as the timer expired
	// closed the same channel twice and panicked the process. A select takes
	// exactly one branch however many triggers fire.
	//
	// The channel is what signal.NotifyContext used to be. NotifyContext
	// cancels on either signal without saying which one fired, and the exit
	// code is 128 plus the number of the one that did (#48) — so the signal has
	// to be read somewhere. Reading it here, rather than from a second channel
	// registered beside NotifyContext, keeps it to a single registration: two
	// of them leave a window between them in which a signal reaches one and not
	// the other. `signal.Stop` is deferred, which is the other half of what #14
	// fixed, and the buffer is what makes a signal arriving before the select
	// is reached a shutdown rather than a lost one.
	//
	// Registered before the first line is printed, so the startup output is
	// itself evidence the handler is installed: until it is, the default
	// disposition of either signal terminates the process outright and there is
	// no shutdown to report. TestExitCodes relies on exactly that.
	received := make(chan os.Signal, 1)
	signal.Notify(received, shutdownSignals...)
	defer signal.Stop(received)

	fmt.Println(s.startupMessage())

	if hint := s.hintMessage(); hint != "" {
		fmt.Println(hint)
	}

	// Started before the deadline is set, so the elapsed time the summary
	// reports is never less than the timeout the operator asked for.
	start := time.Now()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	if s.timeout > 0 {
		var expire context.CancelFunc
		ctx, expire = context.WithTimeout(ctx, s.timeout)
		defer expire()
	}

	var wg sync.WaitGroup

	// One slot per worker, written by the worker that owns it and read by
	// whoever is reporting: the summary below, once wg.Wait() has ordered the
	// two, and the --report heartbeat in waitForShutdown while the workers are
	// still hashing. That second reader is what makes the slots atomic rather
	// than the plain uint64s #49 wrote once at the end — read mid-run, a plain
	// slot is a data race, and one no existing test would have provoked (#70).
	//
	// Still per-worker rather than one shared counter, and for a reason that
	// survives the change: every slot has exactly one writer, so publishing a
	// count is a store of the worker's own local rather than a read-modify-write
	// over a value every other worker is also incrementing. Not a claim about
	// cache lines — these slots are eight bytes and unpadded, so eight workers'
	// counters share one — and at one hash per ~0.18s per worker there is
	// nothing there to measure either way. What it keeps is that no worker's
	// count is a number another worker also writes.
	hashes := make([]atomic.Uint64, s.workers)

	wg.Add(s.workers)
	for i := range s.workers {
		go func() {
			defer wg.Done()
			s.stressTestCPU(ctx, &hashes[i])
		}()
	}

	sig := s.waitForShutdown(ctx, received, hashes, start)

	fmt.Println(shutdownMessage(sig))

	// Whichever branch ran, the workers stop here: on the signal path this is
	// what tells them, and on the timer path ctx is already done and this is a
	// no-op.
	stop()

	wg.Wait()

	fmt.Println(s.summaryMessage(totalHashes(hashes), time.Since(start)))

	if sig != nil {
		return &SignalError{Signal: sig}
	}

	return nil
}

// waitForShutdown blocks until the run ends, and prints a progress line every
// report interval while it waits. It returns the signal that ended the run, or
// nil where the deadline expired — the distinction Run's shutdown line and the
// process exit code are both chosen from.
//
// The wait was this select without its third branch, inline in Run, and that is
// the whole of what #70 is about: with both other branches blocking, nothing at
// all executed on this goroutine between the startup line and the shutdown
// line. A thirty-minute Kubernetes Job showed one line in `kubectl logs` for
// the twenty-nine minutes it was working, the other two arriving only once it
// had stopped, so an operator could not tell a healthy pegged pod from a wedged
// process without leaving the logs. The image is FROM scratch, so there is no
// shell to `kubectl exec` in with: stdout is the only in-band window there is.
func (s *Stressy) waitForShutdown(ctx context.Context, received <-chan os.Signal, hashes []atomic.Uint64, start time.Time) os.Signal {
	// nil where --report is off, and a receive from a nil channel blocks
	// forever — so the default run waits on exactly the two channels it waited
	// on before this existed, rather than on a ticker that has to be given an
	// interval nothing wants.
	var tick <-chan time.Time

	if s.report > 0 {
		ticker := time.NewTicker(s.report)
		defer ticker.Stop()

		tick = ticker.C
	}

	for {
		select {
		case sig := <-received:
			return sig
		case <-ctx.Done():
			// Nothing else cancels ctx before this point — both of Run's cancel
			// functions are deferred — so reaching this branch means the
			// deadline expired. With no timeout configured ctx has no deadline
			// and the select waits on the signal alone. Returning nil is what
			// tells the two shutdowns apart from here on.
			return nil
		case <-tick:
			// time.Since rather than the timestamp the tick carries. A tick
			// delivered late carries the time it fired, which would print the
			// elapsed time the line would have had if the process were
			// healthy — hiding exactly the pathology an operator turns this on
			// to see.
			fmt.Println(progressMessage(totalHashes(hashes), time.Since(start)))
		}
	}
}

// totalHashes is what the workers have computed between them: the one place the
// per-worker slots are added up, read both by the heartbeat mid-run and by the
// summary once every worker has drained.
//
// Indexed rather than ranged over by value, because an atomic.Uint64 must not
// be copied — `for _, n := range hashes` compiles and go vet's copylocks
// analyser is what stops it.
func totalHashes(hashes []atomic.Uint64) uint64 {
	var total uint64

	for i := range hashes {
		total += hashes[i].Load()
	}

	return total
}

// startupMessage is the line Run prints before the workers start: how many of
// them, and for how long. It is the first thing every user of this tool sees,
// and it used to open with "1 workers" on the default run (#17c).
//
// Built as a string rather than printed in place so that it is testable
// without capturing os.Stdout.
func (s *Stressy) startupMessage() string {
	// The timeout is a time.Duration and formats itself — "30s", "5m0s" — so
	// the other half of #17c, "1 seconds", went with the duration flag in #26.
	duration := "indefinitely"
	if s.timeout > 0 {
		duration = "for " + s.timeout.String()
	}

	return fmt.Sprintf("Starting CPU stress test with %d %s %s", s.workers, plural(s.workers, "worker", "workers"), duration)
}

// hintMessage is the second line Run prints, when there is one worth printing.
//
// `Use --help for additional information` printed on every run, bounded ones
// included — every scripted invocation and every Kubernetes Job log the
// README's workflow produces, where the reader has already configured the run
// and the pointer is noise. Meanwhile the hint an indefinite run actually needs
// was absent: a bare `stressy` announced it would run "indefinitely" and never
// said how to stop it (#52).
//
// So the line is now the indefinite run's alone, and leads with the stop
// instruction. Both spellings of it are real: Ctrl-C is SIGINT at a terminal,
// and SIGTERM is what a `docker stop`, a `kubectl delete pod` and a node drain
// send — which is the case the terminal wording alone would leave out, and the
// one this tool is most often deployed into. The `--help` pointer is kept here
// and only here, because a bare `stressy` is where a user most plausibly has
// not read anything, and in the `FROM scratch` image `--help` is the only
// documentation that ships (#27c).
//
// Built as a string rather than printed in place, for the same reason
// startupMessage is: a sibling function rather than a second line folded into
// that one, so that each is checked for what it says on its own.
func (s *Stressy) hintMessage() string {
	if s.timeout > 0 {
		return ""
	}

	return "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information"
}

// progressMessage is the line a run started with --report prints on every tick:
// how long it has been going, how much work it has done, and at what rate.
//
// The rate is cumulative — every hash since the run started, over the whole
// elapsed time — rather than what the last interval alone managed. Two reasons.
// It is the same figure summaryMessage prints, so the last progress line of a
// run and the summary under it agree instead of differing by a windowing choice
// the operator cannot see; and it is the figure the README tells people to
// compare across node pools, which wants the average rather than whatever the
// last thirty seconds did.
//
// The elapsed time is rounded the way the summary rounds it, and deliberately
// not to the tick interval: rounding to the interval would print "5m0s" for a
// tick that arrived at 7m, which is the one thing a heartbeat must not do.
//
// A plain function rather than a method, like shutdownMessage: the line says
// nothing about the configuration the run was started with. Built as a string
// rather than printed in place, for the same reason startupMessage is.
func progressMessage(hashes uint64, elapsed time.Duration) string {
	return fmt.Sprintf(
		"%s elapsed, %d %s, %.1f hashes/s",
		elapsed.Round(time.Millisecond),
		hashes, plural(hashes, "hash", "hashes"),
		hashRate(hashes, elapsed),
	)
}

// shutdownMessage is the line Run prints once the run is ending: why it is
// ending. sig is the signal that stopped it, or nil where the timer expired —
// the same distinction Run's select has just made, and the same one the exit
// code is chosen from further out (#48).
//
// Built as a string rather than printed in place, for the same reason
// startupMessage is: printed in place, the two branches were exercised by every
// shutdown test in this package and read by none of them, so swapping the
// messages left this package green. What objected was TestExitCodes, which
// reads them off a real child process and is not built on Windows — a whole
// process for a string. A run that hit its timer would otherwise tell everyone
// reading pod logs it had been signalled: a line an operator reasonably treats
// as evidence about eviction, saying the opposite of what happened (#57).
func shutdownMessage(sig os.Signal) string {
	if sig == nil {
		return "Timer expired, shutting down..."
	}

	return "Received signal, shutting down..."
}

// summaryMessage is the line Run prints once every worker has drained: what the
// run actually did, where the line above it says only why it stopped. Before
// it, "Timer expired, shutting down..." was a finished run's last word — the
// ellipsis promised something the process never printed, and an operator got no
// confirmation the workers had worked and no number to compare across machines
// (#49).
//
// That number is worth having. bcrypt at a fixed cost is constant work per
// hash, so hashes per second is a crude but legitimate cross-node benchmark:
// run the same Job on every node pool and a node hashing 30% slower is a
// finding. It is also what makes an interrupted run legible, since a partial
// count against a full timeout is the shape of a run that was cut short.
//
// The elapsed time is measured rather than the configured timeout echoed back:
// Run waits for the hash each worker is inside, so a run ends up to one hash
// past its deadline, and the rate has to divide by the time that passed. Built
// as a string rather than printed in place, for the same reason startupMessage
// is.
func (s *Stressy) summaryMessage(hashes uint64, elapsed time.Duration) string {
	return fmt.Sprintf(
		"Computed %d %s in %s (%.1f hashes/s, %d %s)",
		hashes, plural(hashes, "hash", "hashes"),
		// Rounded because the digits below a millisecond are noise against a
		// hash that costs two hundred of them, and left to Duration to format
		// so that the summary spells a duration the way the startup line does.
		elapsed.Round(time.Millisecond),
		hashRate(hashes, elapsed),
		s.workers, plural(s.workers, "worker", "workers"),
	)
}

// hashRate is the rate both reporting lines quote: hashes over the seconds they
// took. One function rather than the same division in each, so that the
// heartbeat and the summary cannot come to disagree about what the number means.
//
// The guard is the reason it is worth a function at all. A run too short to
// measure is not a thing that happens — the clock is monotonic and one hash
// costs ~0.18s — but dividing by it would print "+Inf hashes/s" if it did.
func hashRate(hashes uint64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}

	return float64(hashes) / elapsed.Seconds()
}

// plural picks the form of a noun that goes with n. The startup line opened
// every default run with "1 workers" for the life of the project (#17c), and
// the summary line has two counts of its own to get right.
func plural[T int | uint64](n T, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// ValidateWorkers reports whether workers is a count a run can be started with.
//
// Exported, like the other validators here, because the command layer checks
// each of them before the run — where it still knows whether a value came from
// `-w` or from STRESSY_WORKERS, and can therefore name the one that produced
// it. This package cannot: by the time it holds the value, the flag, the
// variable and the default are one int (#51). The rule lives here so that the
// two callers cannot come to disagree about it.
func ValidateWorkers(workers int) error {
	if workers < 1 {
		return fmt.Errorf("workers must be 1 or greater")
	}

	return nil
}

// ValidateTimeout reports whether timeout is a duration a run can be started
// with. Zero is valid and means indefinite; see ValidateWorkers for why this is
// exported.
func ValidateTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("timeout must be 0 (indefinite) or greater")
	}

	return nil
}

// ValidateReport reports whether report is an interval a run can be started
// with. Zero is valid and means no progress line at all, which is the default;
// see ValidateWorkers for why this is exported.
//
// No upper bound and no lower one beyond zero. An interval longer than the
// timeout prints nothing, which is what asking for a report every hour on a
// five-minute run means, and an interval of a millisecond floods stdout — but
// both are what the operator typed, and this flag is off unless they typed
// something.
func ValidateReport(report time.Duration) error {
	if report < 0 {
		return fmt.Errorf("report must be 0 (off) or greater")
	}

	return nil
}

// validateConfig checks if the Stressy instance's configuration is valid.
// Returns an error if:
//   - workers is less than 1
//   - timeout is negative
//   - report is negative
//
// The command validates the same three settings before it gets here, so on that
// path this is a backstop rather than the first line of defence. It is still
// where the rules are enforced for every other caller of Run, and it runs
// before a single worker starts.
func (s *Stressy) validateConfig() error {
	if err := ValidateWorkers(s.workers); err != nil {
		return err
	}

	if err := ValidateTimeout(s.timeout); err != nil {
		return err
	}

	return ValidateReport(s.report)
}

// stressTestCPU performs CPU-intensive operations in a loop, publishing how
// many hashes it has computed into hashes as it goes. It runs in its own
// goroutine and continues until ctx is cancelled. The CPU load is generated by
// repeatedly computing bcrypt hashes at hashCost, which is where the whole of
// the load lives — bcrypt salts every call itself, so nothing outside the hash
// has to vary for the work to be real, and a worker returns within one hash of
// ctx being done.
//
// The running count is a plain local that nothing else can see, and publishing
// it is a separate step: a store the worker owns, into a slot no other worker
// writes. That is what #49's returned count bought — nothing about counting the
// work touches the work — kept while giving the --report heartbeat something to
// read before the run is over (#70).
func (s *Stressy) stressTestCPU(ctx context.Context, hashes *atomic.Uint64) {
	// Hoisted: formatting a timestamp per iteration was never the load this
	// tool exists to generate, and a constant keeps the input well inside
	// bcrypt's 72-byte limit rather than merely under it by accident.
	password := []byte("stressy")

	var computed uint64

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

			computed++
			hashes.Store(computed)
		}
	}
}
