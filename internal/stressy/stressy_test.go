package stressy

import (
	"context"
	"testing"
	"time"
)

// stopBudget bounds how long a worker may take to observe cancellation. It is
// far looser than the ~0.2s a single hash at hashCost costs, because it has to
// hold on a loaded CI runner under -race, where that hash measures ~1.9s. It
// is still three orders of magnitude under the ~26 hours bcrypt.MaxCost took,
// which is the bug it guards (#15).
const stopBudget = 30 * time.Second

func TestNew(t *testing.T) {
	s := New(Cfg{Workers: 4, Timeout: 30 * time.Second})

	if s.workers != 4 {
		t.Errorf("workers = %d, want 4", s.workers)
	}
	if s.timeout != 30*time.Second {
		t.Errorf("timeout = %s, want %s", s.timeout, 30*time.Second)
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
		{name: "zero workers", cfg: Cfg{Workers: 0, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative workers", cfg: Cfg{Workers: -1, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative timeout", cfg: Cfg{Workers: 1, Timeout: -time.Second}, wantErr: "timeout must be 0 (indefinite) or greater"},
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
	}{
		{
			name: "cancelled before the worker starts",
			ctx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				t.Cleanup(cancel)
				return ctx
			},
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
			done := make(chan struct{})
			go func() {
				defer close(done)
				New(Cfg{Workers: 1}).stressTestCPU(tt.ctx(t))
			}()

			select {
			case <-done:
			case <-time.After(stopBudget):
				t.Fatalf("stressTestCPU did not return within %s of cancellation", stopBudget)
			}
		})
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
