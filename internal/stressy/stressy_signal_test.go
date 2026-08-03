//go:build unix

// Signalling the test binary itself is the only way to reach Run's own handler
// from in-process, and sending SIGINT to your own process is not supported on
// Windows. Releases cross-compile there, so the test is kept out of that build
// rather than left to fail in it.

package stressy

import (
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
func TestRunStopsOnSignal(t *testing.T) {
	tests := []struct {
		name    string
		sig     syscall.Signal
		timeout time.Duration
	}{
		{name: "SIGINT", sig: syscall.SIGINT},
		{name: "SIGTERM", sig: syscall.SIGTERM},
		// The #14 race itself: a signal arriving as the timer expires used to
		// be two unsynchronised closes of one channel. Both now cancel the
		// same context, which cancels once.
		{name: "SIGINT racing the timeout", sig: syscall.SIGINT, timeout: time.Millisecond},
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
					if err != nil {
						t.Fatalf("Run() error = %v, want nil", err)
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
