package stressy

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
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

// progressLine is the shape a --report tick prints; its numbers are measured.
var progressLine = regexp.MustCompile(`^\S+ elapsed, (\d+) hash(?:es)?, \d+\.\d+ hashes/s$`)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		wantErr string
	}{
		{name: "one worker, indefinite", cfg: Cfg{Workers: 1, Timeout: 0}},
		{name: "one worker, one second", cfg: Cfg{Workers: 1, Timeout: time.Second}},
		{name: "many workers", cfg: Cfg{Workers: 64, Timeout: time.Minute}},
		{name: "sub-second timeout", cfg: Cfg{Workers: 1, Timeout: 250 * time.Millisecond}},
		{name: "reporting off", cfg: Cfg{Workers: 1, Timeout: 30 * time.Second, Report: 0}},
		{name: "reporting on", cfg: Cfg{Workers: 1, Timeout: time.Minute, Report: 30 * time.Second}},
		{name: "report at the floor", cfg: Cfg{Workers: 1, Timeout: time.Minute, Report: reportFloor}},
		// The boundary #115 leaves open: one tick, landing on the deadline.
		{name: "report as long as the run", cfg: Cfg{Workers: 1, Timeout: time.Second, Report: time.Second}},
		// An indefinite run outlives every interval, so none is too long for it.
		{name: "a long report on an indefinite run", cfg: Cfg{Workers: 1, Timeout: 0, Report: time.Hour}},
		{name: "zero workers", cfg: Cfg{Workers: 0, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative workers", cfg: Cfg{Workers: -1, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative timeout", cfg: Cfg{Workers: 1, Timeout: -time.Second}, wantErr: "timeout must be 0 (indefinite) or greater"},
		{name: "negative report", cfg: Cfg{Workers: 1, Report: -time.Second}, wantErr: "report must be 0 (off) or 1s or greater"},
		// #114: `-t 1s -r 1ns` put hundreds of thousands of lines on stdout.
		{name: "report of a nanosecond", cfg: Cfg{Workers: 1, Timeout: time.Second, Report: time.Nanosecond}, wantErr: "report must be 0 (off) or 1s or greater"},
		{name: "report just under the floor", cfg: Cfg{Workers: 1, Timeout: time.Minute, Report: reportFloor - time.Nanosecond}, wantErr: "report must be 0 (off) or 1s or greater"},
		// #115: three lines and exit 0, which is what `-r 1s` mistyped looks like.
		{name: "report longer than the run", cfg: Cfg{Workers: 1, Timeout: 3 * time.Second, Report: time.Minute}, wantErr: "report 1m0s is longer than timeout 3s, so no progress line would print"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validate() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("validate() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestStartupMessage covers #17c: the default run announced itself as "1 workers".
func TestStartupMessage(t *testing.T) {
	tests := []struct {
		name string
		cfg  Cfg
		want string
	}{
		{name: "one worker", cfg: Cfg{Workers: 1}, want: "Starting CPU stress test with 1 worker indefinitely"},
		{name: "several workers", cfg: Cfg{Workers: 4}, want: "Starting CPU stress test with 4 workers indefinitely"},
		{name: "one worker, bounded", cfg: Cfg{Workers: 1, Timeout: 5 * time.Minute}, want: "Starting CPU stress test with 1 worker for 5m0s"},
		{name: "several workers, bounded", cfg: Cfg{Workers: 4, Timeout: 30 * time.Second}, want: "Starting CPU stress test with 4 workers for 30s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.startupMessage(); got != tt.want {
				t.Errorf("startupMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHintMessage covers #52: the pointer printed always, the stop hint never.
func TestHintMessage(t *testing.T) {
	tests := []struct {
		name string
		cfg  Cfg
		want string
	}{
		{name: "indefinite", cfg: Cfg{Workers: 1}, want: "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information"},
		{name: "indefinite, several workers", cfg: Cfg{Workers: 4}, want: "Press Ctrl+C or send SIGTERM to stop. Use --help for additional information"},
		{name: "bounded", cfg: Cfg{Workers: 1, Timeout: 5 * time.Minute}},
		{name: "bounded, sub-second", cfg: Cfg{Workers: 4, Timeout: 250 * time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.hintMessage(); got != tt.want {
				t.Errorf("hintMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHintMessageNamesEverySignalARunStopsOn holds the hint to the signal list.
func TestHintMessageNamesEverySignalARunStopsOn(t *testing.T) {
	spellings := map[os.Signal]string{
		syscall.SIGINT:  "Ctrl+C",
		syscall.SIGTERM: "SIGTERM",
	}

	hint := Cfg{Workers: 1}.hintMessage()

	for _, sig := range shutdownSignals {
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

// TestProgressMessage covers #70's line: a count to spell and a rate to divide.
func TestProgressMessage(t *testing.T) {
	tests := []struct {
		name    string
		hashes  uint64
		elapsed time.Duration
		want    string
	}{
		{name: "a minute in", hashes: 1320, elapsed: 60001 * time.Millisecond, want: "1m0.001s elapsed, 1320 hashes, 22.0 hashes/s"},
		{name: "one hash", hashes: 1, elapsed: 200 * time.Millisecond, want: "200ms elapsed, 1 hash, 5.0 hashes/s"},
		{name: "no hash finished yet", hashes: 0, elapsed: 50 * time.Millisecond, want: "50ms elapsed, 0 hashes, 0.0 hashes/s"},
		{name: "elapsed time is rounded", hashes: 11, elapsed: 2*time.Second + 1499*time.Microsecond, want: "2.001s elapsed, 11 hashes, 5.5 hashes/s"},
		// A late tick says when it fired; the interval never reaches this function.
		{name: "a tick delivered late", hashes: 1650, elapsed: 7 * time.Minute, want: "7m0s elapsed, 1650 hashes, 3.9 hashes/s"},
		{name: "no time passed at all", hashes: 0, elapsed: 0, want: "0s elapsed, 0 hashes, 0.0 hashes/s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := progressMessage(tt.hashes, tt.elapsed)

			if got != tt.want {
				t.Errorf("progressMessage(%d, %s) = %q, want %q", tt.hashes, tt.elapsed, got, tt.want)
			}

			// The shape as well as the text: the README shows both.
			if !progressLine.MatchString(got) {
				t.Errorf("progressMessage(%d, %s) = %q, which is not the shape the README documents", tt.hashes, tt.elapsed, got)
			}
		})
	}
}

// TestShutdownMessage covers #57: printed in place, the two lines were swappable.
// The two signalled cases are #111: they printed the same line while exiting 130
// and 143, so a log could not say which shutdown it had recorded.
//
// Every case carries the drain clause, which is #122: the line was the last one
// a SIGKILLed run ever printed, and it said nothing about the wait that was
// about to swallow it. Written out in full rather than built from drainNotice,
// because a want assembled from the code under test is green at whatever the two
// of them agree on.
func TestShutdownMessage(t *testing.T) {
	tests := []struct {
		name string
		// sig is nil where Run's deadline branch was taken.
		sig  os.Signal
		want string
	}{
		{name: "timer expired", want: "Timer expired, shutting down; waiting for every worker to finish the hash it is on..."},
		{name: "SIGINT", sig: syscall.SIGINT, want: "Received SIGINT, shutting down; waiting for every worker to finish the hash it is on..."},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: "Received SIGTERM, shutting down; waiting for every worker to finish the hash it is on..."},
		// Unreachable from shutdownSignals; the alternative is an unnamed signal.
		{name: "a signal with no spelling", sig: unnumberedSignal{}, want: "Received unnumbered, shutting down; waiting for every worker to finish the hash it is on..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shutdownMessage(tt.sig); got != tt.want {
				t.Errorf("shutdownMessage(%v) = %q, want %q", tt.sig, got, tt.want)
			}
		})
	}
}

// TestShutdownMessageNamesEverySignalARunStopsOn holds the line to the signal
// list: a signal added to shutdownSignals with no spelling in signalName would
// print os.Signal.String()'s "terminated" where the exit-code table says SIGTERM.
func TestShutdownMessageNamesEverySignalARunStopsOn(t *testing.T) {
	spellings := map[os.Signal]string{
		syscall.SIGINT:  "SIGINT",
		syscall.SIGTERM: "SIGTERM",
	}

	for _, sig := range shutdownSignals {
		want, ok := spellings[sig]
		if !ok {
			t.Errorf("a run stops on %v, which this test knows no spelling for; add it to spellings and to signalName", sig)

			continue
		}

		if got := shutdownMessage(sig); !strings.HasPrefix(got, "Received "+want+", shutting down;") {
			t.Errorf("shutdownMessage(%v) = %q, want it to name the signal %q (#111)", sig, got, want)
		}
	}
}

// TestSummaryMessage covers #49: a finished run used to say nothing it did.
func TestSummaryMessage(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		hashes  uint64
		elapsed time.Duration
		want    string
	}{
		{name: "several workers", cfg: Cfg{Workers: 4, Timeout: time.Minute}, hashes: 1324, elapsed: 60100 * time.Millisecond, want: "Computed 1324 hashes in 1m0.1s (22.0 hashes/s, 4 workers)"},
		{name: "one worker, one hash", cfg: Cfg{Workers: 1, Timeout: 200 * time.Millisecond}, hashes: 1, elapsed: 200 * time.Millisecond, want: "Computed 1 hash in 200ms (5.0 hashes/s, 1 worker)"},
		{name: "interrupted before the first hash", cfg: Cfg{Workers: 2}, hashes: 0, elapsed: 3 * time.Millisecond, want: "Computed 0 hashes in 3ms (0.0 hashes/s, 2 workers)"},
		{name: "elapsed time is rounded", cfg: Cfg{Workers: 1, Timeout: 2 * time.Second}, hashes: 11, elapsed: 2*time.Second + 1499*time.Microsecond, want: "Computed 11 hashes in 2.001s (5.5 hashes/s, 1 worker)"},
		{name: "no time passed at all", cfg: Cfg{Workers: 1}, hashes: 0, elapsed: 0, want: "Computed 0 hashes in 0s (0.0 hashes/s, 1 worker)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.summaryMessage(tt.hashes, tt.elapsed); got != tt.want {
				t.Errorf("summaryMessage(%d, %s) = %q, want %q", tt.hashes, tt.elapsed, got, tt.want)
			}
		})
	}
}

// TestSignalErrorExitCode pins #48's codes: `128 + signum`, which any Job reads.
func TestSignalErrorExitCode(t *testing.T) {
	tests := []struct {
		name string
		sig  os.Signal
		want int
		// wantMsg is the same spelling the shutdown line uses (#111).
		wantMsg string
	}{
		{name: "SIGINT", sig: syscall.SIGINT, want: 130, wantMsg: "run interrupted by SIGINT"},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: 143, wantMsg: "run interrupted by SIGTERM"},
		// Unreachable from shutdownSignals; the alternative is a panic (#14).
		{name: "a signal with no number", sig: unnumberedSignal{}, want: 1, wantMsg: "run interrupted by unnumbered"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &SignalError{Signal: tt.sig}

			if got := err.ExitCode(); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

// unnumberedSignal is the only input ExitCode cannot map.
type unnumberedSignal struct{}

func (unnumberedSignal) String() string { return "unnumbered" }
func (unnumberedSignal) Signal()        {}

// TestRunRejectsInvalidConfig covers Run's gate, which fails before any worker starts.
func TestRunRejectsInvalidConfig(t *testing.T) {
	if err := (Cfg{Workers: 0}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}

	if err := (Cfg{Workers: 1, Timeout: -time.Second}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}
}

// TestStressTestCPUStopsWhenCancelled is #15: the check ran once every ~26 hours.
func TestStressTestCPUStopsWhenCancelled(t *testing.T) {
	tests := []struct {
		name string
		// ctx is cancelled before the worker reads it, or mid-hash.
		ctx func(t *testing.T) context.Context
		// wantNoHashes is set where the count is decided rather than raced.
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
			// One writer, no reader until it returns, so the read below is safe.
			var hashes atomic.Uint64

			done := make(chan struct{})
			go func() {
				defer close(done)

				Cfg{Workers: 1}.stressTestCPU(tt.ctx(t), &hashes)
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

// TestStressTestCPUPublishesAsItGoes: the count has to be readable mid-run (#70).
func TestStressTestCPUPublishesAsItGoes(t *testing.T) {
	var hashes atomic.Uint64

	// Cancelled on a count rather than a deadline, which would be a second budget.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		Cfg{Workers: 1}.stressTestCPU(ctx, &hashes)
	}()

	deadline := time.After(stopBudget)

	for hashes.Load() == 0 {
		select {
		case <-done:
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

// TestRunStopsAtTimeout: Run waits on its workers, so returning is the evidence.
func TestRunStopsAtTimeout(t *testing.T) {
	const timeout = 10 * time.Millisecond

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- Cfg{Workers: 2, Timeout: timeout, Out: io.Discard}.Run() }()

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

// TestRunDefaultOutputIsUnchanged is #70's promise: three lines, nothing between.
func TestRunDefaultOutputIsUnchanged(t *testing.T) {
	const timeout = 10 * time.Millisecond

	var buf bytes.Buffer

	if err := (Cfg{Workers: 1, Timeout: timeout, Out: &buf}).Run(); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	out := buf.String()
	got := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	if len(got) != 3 {
		t.Fatalf("Run() printed %d lines:\n%s\nwant exactly the three a bounded run has always printed (#70)", len(got), out)
	}

	if want := "Starting CPU stress test with 1 worker for 10ms"; got[0] != want {
		t.Errorf("Run() line 1 = %q, want %q", got[0], want)
	}
	if want := "Timer expired, shutting down; waiting for every worker to finish the hash it is on..."; got[1] != want {
		t.Errorf("Run() line 2 = %q, want %q", got[1], want)
	}
	if !strings.HasPrefix(got[2], "Computed ") {
		t.Errorf("Run() line 3 = %q, want the end-of-run summary", got[2])
	}
}

// TestRunPrintsProgressWhenAsked is #70's wiring: the ticker starts and repeats.
func TestRunPrintsProgressWhenAsked(t *testing.T) {
	const (
		// The shortest interval a run accepts since #114, so this is also what
		// holds the floor to a run that still reports at it.
		report = reportFloor
		// Four intervals against an assertion asking for two.
		timeout = 4 * time.Second
	)

	var buf bytes.Buffer

	if err := (Cfg{Workers: 1, Timeout: timeout, Report: report, Out: &buf}).Run(); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	out := buf.String()

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
		case strings.HasPrefix(line, "Timer expired, shutting down;"):
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

	if firstTick < startup || lastTick > shutdown {
		t.Errorf("Run() printed:\n%s\nwant every progress line between the startup line and the shutdown line (#70)", out)
	}
}

// TestWaitForShutdownPrefersAPendingSignal is #117. A signal landing in the same
// instant the deadline expires leaves both cases of the select ready, and Go
// picks between ready cases at random — so a SIGTERM could be reported as
// `Timer expired, ...` and exit 0, where README.md's table says 143 with no
// clause on it. The window is microseconds wide in a real run; here both are
// ready by construction.
//
// Called many times over, because one call proves nothing: the outer select is
// still choosing at random, and a run without the second receive returns the
// signal from about half its calls anyway. Every call is decided by that
// receive, so a hundred of them leave a missed regression at 2^-100 rather than
// at one coin toss.
func TestWaitForShutdownPrefersAPendingSignal(t *testing.T) {
	const calls = 100

	tests := []struct {
		name string
		// pending is queued before the deadline is observed; nil queues none.
		pending os.Signal
		want    os.Signal
	}{
		{name: "a signal pending as the deadline expires", pending: syscall.SIGTERM, want: syscall.SIGTERM},
		{name: "the deadline alone", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range calls {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				received := make(chan os.Signal, 1)
				if tt.pending != nil {
					received <- tt.pending
				}

				var hashes atomic.Uint64

				got := Cfg{Workers: 1, Out: io.Discard}.waitForShutdown(ctx, received, &hashes, time.Now())
				if got != tt.want {
					t.Fatalf("waitForShutdown() = %v on call %d of %d, want %v: a run a signal ended must not be reported as one the timer ended (#117)", got, i+1, calls, tt.want)
				}
			}
		})
	}
}

// TestRunIsReusable covers #14's third item: a second call panicked on a channel.
func TestRunIsReusable(t *testing.T) {
	cfg := Cfg{Workers: 1, Timeout: time.Millisecond, Out: io.Discard}

	for i := range 2 {
		done := make(chan error, 1)
		go func() { done <- cfg.Run() }()

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
