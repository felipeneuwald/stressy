//go:build unix

// Signalling the test binary itself is the only way to reach Run's own handler
// from in-process, and sending SIGINT to your own process is not supported on
// Windows. Releases cross-compile there, so the test is kept out of that build
// rather than left to fail in it.

package stressy

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestRunStopsOnSignal covers the signal half of Run's shutdown, the path that
// raced the timer in #14. It also pins the guarantee that made that race
// possible to remove: whichever of the two fires, the run ends by cancelling
// one context, and ends without a panic.
//
// Since #48 it pins one thing more, and it is the whole of what that issue is
// about: which of the two shutdowns happened has to survive Run returning. It
// used to return nil either way, so an interrupted run and a completed one were
// the same result to every caller, all the way out to the process exit status.
func TestRunStopsOnSignal(t *testing.T) {
	tests := []struct {
		name     string
		sig      syscall.Signal
		timeout  time.Duration
		wantExit int
	}{
		{name: "SIGINT", sig: syscall.SIGINT, wantExit: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, wantExit: 143},
		// The #14 race itself: a signal arriving as the timer expires used to
		// be two unsynchronised closes of one channel. Both now meet in one
		// select, which takes one branch.
		{name: "SIGINT racing the timeout", sig: syscall.SIGINT, timeout: time.Millisecond, wantExit: 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Registering here as well keeps the test binary alive if a signal
			// lands before Run installs its own handler: with no handler at
			// all, the default disposition terminates the process. Held until
			// after the sending loop below has stopped.
			guard := make(chan os.Signal, 1)
			signal.Notify(guard, tt.sig)
			defer signal.Stop(guard)

			done := make(chan error, 1)
			go func() { done <- New(Cfg{Workers: 1, Timeout: tt.timeout}).Run() }()

			// Run installs its handler asynchronously, so signal until it takes.
			deadline := time.After(stopBudget)
			for {
				if err := syscall.Kill(syscall.Getpid(), tt.sig); err != nil {
					t.Fatalf("Kill(%v) error = %v", tt.sig, err)
				}

				select {
				case err := <-done:
					sigErr, ok := errors.AsType[*SignalError](err)
					if !ok {
						if err != nil {
							t.Fatalf("Run() error = %v, want a *SignalError", err)
						}

						// nil is the completed run, which only the case with a
						// timeout can produce: there the 1ms timer and the
						// signal are racing on purpose and either winning is a
						// correct answer. With no timeout a signal is the only
						// thing that could have ended the run, so nil would be
						// the #48 bug itself.
						if tt.timeout == 0 {
							t.Fatal("Run() error = nil after a signal, want a *SignalError: an interrupted run must not look like a completed one (#48)")
						}

						return
					}

					if sigErr.Signal != tt.sig {
						t.Errorf("Run() signal = %v, want %v", sigErr.Signal, tt.sig)
					}
					if got := sigErr.ExitCode(); got != tt.wantExit {
						t.Errorf("ExitCode() = %d, want %d", got, tt.wantExit)
					}

					return
				case <-deadline:
					t.Fatalf("Run() did not return within %s of %v", stopBudget, tt.sig)
				case <-time.After(50 * time.Millisecond):
				}
			}
		})
	}
}
