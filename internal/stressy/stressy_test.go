package stressy

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// stopBudget bounds how long a worker may take to observe cancellation. It is
// far looser than the ~0.2s a single hash at hashCost costs, because it has to
// hold on a loaded CI runner under -race, where that hash measures ~1.9s. It
// is still three orders of magnitude under the ~26 hours bcrypt.MaxCost took,
// which is the bug it guards (#15).
const stopBudget = 30 * time.Second

// progressLine is the shape of the line a --report run prints on every tick
// (#70). The numbers in it are measured, so what a test can hold it to is its
// shape — the same trade docs_test.go's summaryLine makes, and for the same
// reason: the README quotes this line back to the reader.
var progressLine = regexp.MustCompile(`^\S+ elapsed, (\d+) hash(?:es)?, \d+\.\d+ hashes/s$`)

func TestNew(t *testing.T) {
	s := New(Cfg{Workers: 4, Timeout: 30 * time.Second, Report: 5 * time.Second})

	if s.workers != 4 {
		t.Errorf("workers = %d, want 4", s.workers)
	}
	if s.timeout != 30*time.Second {
		t.Errorf("timeout = %s, want %s", s.timeout, 30*time.Second)
	}
	if s.report != 5*time.Second {
		t.Errorf("report = %s, want %s", s.report, 5*time.Second)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		wantErr string
	}{
		{name: "one worker, indefinite", cfg: Cfg{Workers: 1, Timeout: 0}},
		{name: "one worker, one second", cfg: Cfg{Workers: 1, Timeout: time.Second}},
		{name: "many workers", cfg: Cfg{Workers: 64, Timeout: time.Minute}},
		// Only expressible since #26 made Timeout a time.Duration; a
		// sub-second timeout used to round to the indefinite 0.
		{name: "sub-second timeout", cfg: Cfg{Workers: 1, Timeout: 250 * time.Millisecond}},
		// Zero is the default and means no progress line at all, which is what
		// keeps a default run's output what it has always been (#70). Paired
		// with a timeout so that it is not the "one worker, indefinite" case
		// above spelled a second way.
		{name: "reporting off", cfg: Cfg{Workers: 1, Timeout: 30 * time.Second, Report: 0}},
		{name: "reporting on", cfg: Cfg{Workers: 1, Timeout: time.Minute, Report: 30 * time.Second}},
		{name: "report longer than the run", cfg: Cfg{Workers: 1, Timeout: time.Second, Report: time.Hour}},
		{name: "zero workers", cfg: Cfg{Workers: 0, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative workers", cfg: Cfg{Workers: -1, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative timeout", cfg: Cfg{Workers: 1, Timeout: -time.Second}, wantErr: "timeout must be 0 (indefinite) or greater"},
		{name: "negative report", cfg: Cfg{Workers: 1, Report: -time.Second}, wantErr: "report must be 0 (off) or greater"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.cfg).validateConfig()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateConfig() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validateConfig() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("validateConfig() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestStartupMessage covers #17c. The default run announced itself as "1
// workers", which is the first line this tool prints and, on a default run,
// the only one until it is interrupted.
func TestStartupMessage(t *testing.T) {
	tests := []struct {
		name string
		cfg  Cfg
		want string
	}{
		{
			name: "one worker",
			cfg:  Cfg{Workers: 1},
			want: "Starting CPU stress test with 1 worker indefinitely",
		},
		{
			name: "several workers",
			cfg:  Cfg{Workers: 4},
			want: "Starting CPU stress test with 4 workers indefinitely",
		},
		{
			name: "one worker, bounded",
			cfg:  Cfg{Workers: 1, Timeout: 5 * time.Minute},
			want: "Starting CPU stress test with 1 worker for 5m0s",
		},
		{
			name: "several workers, bounded",
			cfg:  Cfg{Workers: 4, Timeout: 30 * time.Second},
			want: "Starting CPU stress test with 4 workers for 30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.cfg).startupMessage(); got != tt.want {
				t.Errorf("startupMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHintMessage covers #52. `Use --help for additional information` printed
// on every run, bounded ones included — every scripted invocation and every
// Kubernetes Job log the README's workflow produces — while the hint an
// indefinite run needs was missing: a bare `stressy` said it would run
// "indefinitely" and never said how to stop it.
func TestHintMessage(t *testing.T) {
	tests := []struct {
		name string
		cfg  Cfg
		want string
	}{
		// The line the issue asks for, in full. Its wording is what an operator
		// reads, so it is pinned rather than probed: the stop instruction is a
		// claim the output makes, and per the house rule it gets a test holding
		// it to it.
		{
			name: "indefinite",
			cfg:  Cfg{Workers: 1},
			want: "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information",
		},
		{
			name: "indefinite, several workers",
			cfg:  Cfg{Workers: 4},
			want: "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information",
		},
		// Nothing at all on a bounded run. The operator has already configured
		// it, so the pointer is noise in the one place it is most often read
		// from — a log, after the fact.
		{name: "bounded", cfg: Cfg{Workers: 1, Timeout: 5 * time.Minute}},
		{name: "bounded, sub-second", cfg: Cfg{Workers: 4, Timeout: 250 * time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.cfg).hintMessage(); got != tt.want {
				t.Errorf("hintMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHintMessageNamesEverySignalARunStopsOn keeps the stop instruction honest
// against the list Run registers a handler for. A signal added to
// shutdownSignals is a second way to stop a run, and a line that names one of
// them and not the other tells an operator to reach for a shutdown that is not
// the one their platform offers — the same class of gap as the exit code
// documented nowhere in #48.
func TestHintMessageNamesEverySignalARunStopsOn(t *testing.T) {
	// Ctrl-C is how SIGINT is sent at a terminal, which is the spelling the
	// line uses; anything else is named outright.
	spellings := map[os.Signal]string{
		syscall.SIGINT:  "Ctrl+C",
		syscall.SIGTERM: "SIGTERM",
	}

	hint := New(Cfg{Workers: 1}).hintMessage()

	for _, sig := range ShutdownSignals() {
		want, ok := spellings[sig]
		if !ok {
			t.Errorf("a run stops on %v, which this test knows no wording for; add it to spellings and to hintMessage", sig)

			continue
		}

		if !strings.Contains(hint, want) {
			t.Errorf("hintMessage() = %q, want it to name %v as %q (#52)", hint, sig, want)
		}
	}
}

// TestProgressMessage covers the line #70 adds: the one thing a long run prints
// between its startup lines and its shutdown line, and only when an operator
// asks for it. Table-driven for the reason TestSummaryMessage is — the line
// carries a count that has to be spelled right and a division that has to not
// print "+Inf" — and built as a string so the wording is checked without a
// process to read it off.
func TestProgressMessage(t *testing.T) {
	tests := []struct {
		name    string
		hashes  uint64
		elapsed time.Duration
		want    string
	}{
		{
			name:    "a minute in",
			hashes:  1320,
			elapsed: 60001 * time.Millisecond,
			want:    "1m0.001s elapsed, 1320 hashes, 22.0 hashes/s",
		},
		{
			// Singular, which the startup line got wrong for the life of the
			// project (#17c) and this line gets one chance at per tick.
			name:    "one hash",
			hashes:  1,
			elapsed: 200 * time.Millisecond,
			want:    "200ms elapsed, 1 hash, 5.0 hashes/s",
		},
		{
			// The first tick of a short interval, before any worker has
			// finished the ~0.18s hash it started with. Zero is the report: a
			// heartbeat that skipped it would be silent in exactly the window
			// an operator is watching hardest.
			name:    "no hash finished yet",
			hashes:  0,
			elapsed: 50 * time.Millisecond,
			want:    "50ms elapsed, 0 hashes, 0.0 hashes/s",
		},
		{
			// Rounded like the summary's elapsed time: sub-millisecond digits
			// are noise against a hash costing two hundred of them.
			name:    "elapsed time is rounded",
			hashes:  11,
			elapsed: 2*time.Second + 1499*time.Microsecond,
			want:    "2.001s elapsed, 11 hashes, 5.5 hashes/s",
		},
		{
			// A tick a starved process delivered two minutes late, which says
			// 7m rather than the 5m it was scheduled for. What holds that is
			// structural rather than tested here — the report interval is never
			// passed to this function, so there is nothing it could round to
			// but a millisecond — and this is the case that would have to
			// change if it ever were.
			name:    "a tick delivered late",
			hashes:  1650,
			elapsed: 7 * time.Minute,
			want:    "7m0s elapsed, 1650 hashes, 3.9 hashes/s",
		},
		{
			// Not reachable from a run, whose clock is monotonic, but the rate
			// is a division and this is the divisor that would print "+Inf".
			name:    "no time passed at all",
			hashes:  0,
			elapsed: 0,
			want:    "0s elapsed, 0 hashes, 0.0 hashes/s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := progressMessage(tt.hashes, tt.elapsed)

			if got != tt.want {
				t.Errorf("progressMessage(%d, %s) = %q, want %q", tt.hashes, tt.elapsed, got, tt.want)
			}

			// A copy of the pattern docs_test.go holds the README's sample to,
			// and a copy is all it is: internal/stressy cannot import main, so
			// nothing here would notice that one being edited. What keeps them
			// honest is that each is anchored to real output rather than to
			// the other — this one to the string progressMessage returns, that
			// one to the line TestExitCodes reads off a child process — so a
			// shape that changed on one side fails on the other.
			if !progressLine.MatchString(got) {
				t.Errorf("progressMessage(%d, %s) = %q, which is not the shape the README documents", tt.hashes, tt.elapsed, got)
			}
		})
	}
}

// TestShutdownMessage covers #57. The two lines used to be printed in place,
// inside the select that chooses between them, so every shutdown test in this
// package executed one of them and none read either: swapping the two — a run
// that hit its timer reporting a signal — left this package green, real workers
// and -race included. The only thing that read either line was TestExitCodes,
// which re-execs the binary and reads a child process's stdout, and which is
// not built on Windows. A string that says which of two things happened should
// not need a process to check.
//
// Which of them printed is what a log says about the run after the fact. "Timer
// expired" is a run that served its `-t`; "Received signal" is one that
// something outside the process ended — an eviction, a `docker stop`, a Ctrl-C
// — which is the distinction the exit code carries too (#48), and a wrong line
// here contradicts a right code there.
func TestShutdownMessage(t *testing.T) {
	tests := []struct {
		name string
		// sig is what Run's select leaves behind: the signal that stopped the
		// run, or nil where the deadline branch was taken.
		sig  os.Signal
		want string
	}{
		{name: "timer expired", want: "Timer expired, shutting down..."},
		{name: "SIGINT", sig: syscall.SIGINT, want: "Received signal, shutting down..."},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: "Received signal, shutting down..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shutdownMessage(tt.sig); got != tt.want {
				t.Errorf("shutdownMessage(%v) = %q, want %q", tt.sig, got, tt.want)
			}
		})
	}
}

// TestSummaryMessage covers #49: a finished run used to say nothing about what
// it did, so there was no confirmation the workers had worked and no number to
// compare one machine against another. Table-driven for the same reason
// TestStartupMessage is — the counts in it are the ones that get spelled wrong,
// and the line is built as a string precisely so they can be checked without
// capturing os.Stdout.
func TestSummaryMessage(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		hashes  uint64
		elapsed time.Duration
		want    string
	}{
		{
			name:    "several workers",
			cfg:     Cfg{Workers: 4, Timeout: time.Minute},
			hashes:  1324,
			elapsed: 60100 * time.Millisecond,
			want:    "Computed 1324 hashes in 1m0.1s (22.0 hashes/s, 4 workers)",
		},
		{
			// Both counts singular. The startup line shipped "1 workers" for
			// the life of the project (#17c); this line has two chances at it.
			name:    "one worker, one hash",
			cfg:     Cfg{Workers: 1, Timeout: 200 * time.Millisecond},
			hashes:  1,
			elapsed: 200 * time.Millisecond,
			want:    "Computed 1 hash in 200ms (5.0 hashes/s, 1 worker)",
		},
		{
			// A signal landing before any worker finished its first hash. The
			// zero is the report, not a reason to print nothing.
			name:    "interrupted before the first hash",
			cfg:     Cfg{Workers: 2},
			hashes:  0,
			elapsed: 3 * time.Millisecond,
			want:    "Computed 0 hashes in 3ms (0.0 hashes/s, 2 workers)",
		},
		{
			// The measured elapsed time carries whatever precision the clock
			// gives it, which is noise against a hash costing ~0.18s.
			name:    "elapsed time is rounded",
			cfg:     Cfg{Workers: 1, Timeout: 2 * time.Second},
			hashes:  11,
			elapsed: 2*time.Second + 1499*time.Microsecond,
			want:    "Computed 11 hashes in 2.001s (5.5 hashes/s, 1 worker)",
		},
		{
			// Not reachable from Run, whose clock is monotonic, but the rate is
			// a division and this is the divisor that would print "+Inf".
			name:    "no time passed at all",
			cfg:     Cfg{Workers: 1},
			hashes:  0,
			elapsed: 0,
			want:    "Computed 0 hashes in 0s (0.0 hashes/s, 1 worker)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.cfg).summaryMessage(tt.hashes, tt.elapsed); got != tt.want {
				t.Errorf("summaryMessage(%d, %s) = %q, want %q", tt.hashes, tt.elapsed, got, tt.want)
			}
		})
	}
}

// TestSignalErrorExitCode pins the codes #48 chose, which are the whole of what
// an operator's supervisor sees: `128 + signum` is what timeout(1), the shells
// and every process killed by an unhandled signal report, so a Job or a script
// can read an interrupted run without knowing anything about stressy.
func TestSignalErrorExitCode(t *testing.T) {
	tests := []struct {
		name string
		sig  os.Signal
		want int
	}{
		{name: "SIGINT", sig: syscall.SIGINT, want: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: 143},
		// Nothing in shutdownSignals reaches this, on any platform releases
		// build for. It is here because the alternative to a fallback is a
		// panic during shutdown, which is the failure #14 spent an issue
		// removing.
		{name: "a signal with no number", sig: unnumberedSignal{}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &SignalError{Signal: tt.sig}

			if got := err.ExitCode(); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
			if err.Error() == "" {
				t.Error("Error() = \"\", want it to name the signal")
			}
		})
	}
}

// captureStdout returns everything f prints to os.Stdout.
//
// Run writes its lines with fmt.Println rather than through a writer it is
// configured with, which is why this swaps the file out from under it. That is
// a deliberate trade the rest of this package makes in the other direction:
// every line a run prints is built by a function returning a string, so almost
// nothing here needs a capture. What needs one is the wiring — which lines get
// printed, in which order — and that is worth two tests rather than an io.Writer
// threaded through the package's only exported entry point.
//
// The swap is safe because nothing in this package runs its tests in parallel,
// and it is restored before the captured output is read.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}

	saved := os.Stdout
	os.Stdout = w

	// Drained in the background: a run printing more than the pipe buffer holds
	// would otherwise block inside f with nothing reading the other end.
	captured := make(chan string, 1)

	go func() {
		var buf strings.Builder

		// Reported into the captured output rather than to t: this runs on its
		// own goroutine, which may outlive the test that started it, and every
		// assertion below reads what was captured — so a failure here surfaces
		// as a line no test recognises rather than as silence.
		if _, copyErr := io.Copy(&buf, r); copyErr != nil {
			fmt.Fprintf(&buf, "\nreading the captured output: %v", copyErr)
		}

		captured <- buf.String()
	}()

	f()

	os.Stdout = saved

	if err := w.Close(); err != nil {
		t.Fatalf("closing the capture pipe: %v", err)
	}

	out := <-captured

	if err := r.Close(); err != nil {
		t.Fatalf("closing the capture pipe: %v", err)
	}

	return out
}

// unnumberedSignal is an os.Signal that is not a syscall.Signal, which is the
// only input ExitCode cannot map.
type unnumberedSignal struct{}

func (unnumberedSignal) String() string { return "unnumbered" }
func (unnumberedSignal) Signal()        {}

// TestShutdownSignals covers the list itself, which two things read: Run, to
// register a handler, and TestDocumentedExitCodes, to check that the README
// documents an exit code for every signal in it.
func TestShutdownSignals(t *testing.T) {
	got := ShutdownSignals()

	for _, want := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		if !slices.Contains(got, want) {
			t.Errorf("ShutdownSignals() = %v, want it to include %v", got, want)
		}
	}

	// A copy, not the package's own slice: a caller that sorted or truncated
	// the result would otherwise change which signals a run stops on.
	if len(got) > 0 {
		got[0] = nil

		if ShutdownSignals()[0] == nil {
			t.Error("ShutdownSignals() returns the package's own slice, want a copy")
		}
	}
}

// TestRunRejectsInvalidConfig covers Run's validation gate, which fails before
// any worker goroutine is started.
func TestRunRejectsInvalidConfig(t *testing.T) {
	if err := New(Cfg{Workers: 0}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}

	if err := New(Cfg{Workers: 1, Timeout: -time.Second}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}
}

// TestStressTestCPUStopsWhenCancelled is the direct regression test for #15.
// At bcrypt.MaxCost a worker reached its cancellation check once every ~26
// hours, so the check below was dead code and the shutdown path it belongs to
// could not be exercised at all.
func TestStressTestCPUStopsWhenCancelled(t *testing.T) {
	tests := []struct {
		name string
		// ctx is built per case: one is cancelled before the worker reads it,
		// the other lands while the worker is inside a hash — the case the old
		// code could not survive, since that hash ran for a day.
		ctx func(t *testing.T) context.Context
		// wantNoHashes is set where the count the worker publishes is decided
		// rather than raced: a context already cancelled when the worker reads
		// it buys no hashes, and the summary line for that run has to say 0
		// rather than round the truth up to something that happened (#49).
		wantNoHashes bool
	}{
		{
			name: "cancelled before the worker starts",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				t.Cleanup(cancel)
				return ctx
			},
			wantNoHashes: true,
		},
		{
			name: "cancelled while the worker is hashing",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The counter Run gives a worker: one slot, written by this worker
			// and read by nobody until it has returned, which is what makes
			// reading it below safe without another Load ordering it.
			var hashes atomic.Uint64

			done := make(chan struct{})
			go func() {
				defer close(done)

				New(Cfg{Workers: 1}).stressTestCPU(tt.ctx(t), &hashes)
			}()

			select {
			case <-done:
				if tt.wantNoHashes && hashes.Load() != 0 {
					t.Errorf("stressTestCPU published %d, want 0 from an already-cancelled context", hashes.Load())
				}
			case <-time.After(stopBudget):
				t.Fatalf("stressTestCPU did not return within %s of cancellation", stopBudget)
			}
		})
	}
}

// TestStressTestCPUPublishesAsItGoes is what the heartbeat stands on: the count
// a worker keeps has to be readable while it is still working, not only once it
// has returned. Before #70 it was a plain local returned at the end, so a
// mid-run reader had nothing to read — and a slot filled in only at the end
// would leave every progress line of a long run reporting 0 hashes while the
// machine was visibly pegged.
func TestStressTestCPUPublishesAsItGoes(t *testing.T) {
	var hashes atomic.Uint64

	// Cancelled by this test once it has seen a count, rather than given a
	// deadline of its own. A deadline here would be a second budget for how
	// long one hash may take, and a shorter one than stopBudget: the worker
	// would return empty-handed on a machine slow enough to need the whole of
	// stopBudget, and the failure would read as "published nothing" when what
	// happened was "ran out of time". One budget, and it is the package's.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		New(Cfg{Workers: 1}).stressTestCPU(ctx, &hashes)
	}()

	// Polled rather than slept on: what this asserts is that the count moves
	// before the worker returns, so the read has to happen while it is running.
	deadline := time.After(stopBudget)

	for hashes.Load() == 0 {
		select {
		case <-done:
			// Nothing cancels ctx until the loop is out, so the worker
			// returning at all means it returned without publishing.
			t.Fatal("stressTestCPU returned before it published a hash, so nothing can read its count mid-run (#70)")
		case <-deadline:
			t.Fatalf("stressTestCPU published no hash within %s while still running (#70)", stopBudget)
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()

	select {
	case <-done:
	case <-time.After(stopBudget):
		t.Fatalf("stressTestCPU did not return within %s of cancellation", stopBudget)
	}
}

// TestRunStopsAtTimeout exercises the timeout half of Run's shutdown. Run
// waits on its workers, so its returning is itself the evidence that every
// worker observed the cancelled context.
func TestRunStopsAtTimeout(t *testing.T) {
	const timeout = 10 * time.Millisecond

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- New(Cfg{Workers: 2, Timeout: timeout}).Run() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed < timeout {
			t.Errorf("Run() returned after %s, want it to wait out the %s timeout", elapsed, timeout)
		}
	case <-time.After(stopBudget):
		t.Fatalf("Run() did not return within %s of its %s timeout", stopBudget, timeout)
	}
}

// TestRunDefaultOutputIsUnchanged is the promise #70 is allowed to land on: a
// run nobody asked for a report from prints what it has always printed, and the
// new flag costs the default output nothing. Three lines for a bounded run —
// what it is starting, why it stopped, what it did — and nothing between them,
// which is both the property this project has pinned repeatedly and the
// complaint the issue opens with.
func TestRunDefaultOutputIsUnchanged(t *testing.T) {
	const timeout = 10 * time.Millisecond

	out := captureStdout(t, func() {
		if err := New(Cfg{Workers: 1, Timeout: timeout}).Run(); err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	})

	got := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	if len(got) != 3 {
		t.Fatalf("Run() printed %d lines:\n%s\nwant exactly the three a bounded run has always printed (#70)", len(got), out)
	}

	if want := "Starting CPU stress test with 1 worker for 10ms"; got[0] != want {
		t.Errorf("Run() line 1 = %q, want %q", got[0], want)
	}
	if want := "Timer expired, shutting down..."; got[1] != want {
		t.Errorf("Run() line 2 = %q, want %q", got[1], want)
	}
	if !strings.HasPrefix(got[2], "Computed ") {
		t.Errorf("Run() line 3 = %q, want the end-of-run summary", got[2])
	}
}

// TestRunPrintsProgressWhenAsked is the other half of #70, at the level the
// issue is about: with an interval set, the run says something while it is
// running. progressMessage is checked on its own above; what this adds is the
// wiring — that the ticker is started, that its line reaches stdout, and that it
// happens more than once, which is the difference between a heartbeat and a
// one-off note.
//
// It captures os.Stdout, where the rest of this package deliberately tests
// strings instead. There is no string to test here: what would regress is the
// select branch, the ticker, or the order the three kinds of line come in, and
// none of that is observable from a return value.
func TestRunPrintsProgressWhenAsked(t *testing.T) {
	const (
		report = 20 * time.Millisecond
		// A hundred intervals' worth, against an assertion that asks for two.
		// The margin is the point: this is a statement about recurrence, and it
		// has to hold on a runner where the ticker's goroutine is competing
		// with every other job on the box. A 300ms window did not — two of
		// fifteen ticks is not enough slack to survive one scheduling stall,
		// and it was seen printing a single line under load.
		timeout = 2 * time.Second
	)

	out := captureStdout(t, func() {
		if err := New(Cfg{Workers: 1, Timeout: timeout, Report: report}).Run(); err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	})

	var (
		progress  int
		startup   = -1
		shutdown  = -1
		firstTick = -1
		lastTick  = -1
	)

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "Starting CPU stress test"):
			startup = i
		case line == "Timer expired, shutting down...":
			shutdown = i
		case progressLine.MatchString(line):
			progress++

			if firstTick < 0 {
				firstTick = i
			}

			lastTick = i
		case strings.HasPrefix(line, "Computed "):
		default:
			t.Errorf("Run() printed %q, which is none of the lines a run is documented to print", line)
		}
	}

	if progress < 2 {
		t.Fatalf("Run() with a %s report interval over %s printed:\n%s\nwant a progress line on every tick (#70)", report, timeout, out)
	}

	// Between the startup line and the shutdown line, which is the gap the
	// issue is named after: a heartbeat printed after the run had stopped would
	// tell an operator watching `kubectl logs` nothing they did not already
	// have from the summary.
	if firstTick < startup || lastTick > shutdown {
		t.Errorf("Run() printed:\n%s\nwant every progress line between the startup line and the shutdown line (#70)", out)
	}
}

// TestRunIsReusable covers the third item of #14: Run closed a channel created
// in New, so a second call panicked immediately on the already-closed channel.
// The context replacing it is built per call.
func TestRunIsReusable(t *testing.T) {
	s := New(Cfg{Workers: 1, Timeout: time.Millisecond})

	for i := range 2 {
		done := make(chan error, 1)
		go func() { done <- s.Run() }()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run() call %d error = %v, want nil", i+1, err)
			}
		case <-time.After(stopBudget):
			t.Fatalf("Run() call %d did not return within %s", i+1, stopBudget)
		}
	}
}
