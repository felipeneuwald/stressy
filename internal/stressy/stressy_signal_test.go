//go:build unix

// Signalling this binary is the only in-process way to reach Run's own handler,
// and Windows, which releases build for, does not support it.

package stressy

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestRunStopsOnSignal covers #14's race and #48's which-shutdown guarantee.
func TestRunStopsOnSignal(t *testing.T) {
	tests := []struct {
		name     string
		sig      syscall.Signal
		timeout  time.Duration
		wantExit int
	}{
		{name: "SIGINT", sig: syscall.SIGINT, wantExit: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, wantExit: 143},
		// The #14 race itself: two closes of one channel, now one select.
		{name: "SIGINT racing the timeout", sig: syscall.SIGINT, timeout: time.Millisecond, wantExit: 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keeps the binary alive if a signal beats Run's own handler.
			guard := make(chan os.Signal, 1)
			signal.Notify(guard, tt.sig)
			defer signal.Stop(guard)

			done := make(chan error, 1)
			go func() { done <- Cfg{Workers: 1, Timeout: tt.timeout}.Run() }()

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

						// Only a timeout case can produce nil: it races on purpose.
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
