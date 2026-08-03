package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/flag"
	"github.com/felipeneuwald/stressy/internal/stressy"
)

// newTestCmd builds a command over the real flag definitions and the real
// configuration resolution, but with a no-op RunE: the actual RunE starts the
// stress test, which saturates the CPU and blocks until signalled.
func newTestCmd(t *testing.T, cfg *stressy.Cfg) *cobra.Command {
	t.Helper()

	flags := newFlags(cfg)
	c := &cobra.Command{
		Use:               "stressy",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		SilenceUsage:      true,
		SilenceErrors:     true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return bindAndValidate(cmd, flags)
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	if err := flag.Load(c, flags); err != nil {
		t.Fatalf("flag.Load() error = %v, want nil", err)
	}

	c.SetOut(io.Discard)
	c.SetErr(io.Discard)

	return c
}

func TestConfigResolution(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		args        []string
		wantWorkers int
		wantTimeout time.Duration
	}{
		{
			name:        "defaults",
			wantWorkers: 1,
			wantTimeout: 0,
		},
		{
			name:        "long flags",
			args:        []string{"--workers", "4", "--timeout", "30"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "short flags",
			args:        []string{"-w", "4", "-t", "30"},
			wantWorkers: 4,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			wantWorkers: 8,
			wantTimeout: 60 * time.Second,
		},
		{
			name:        "flags take precedence over environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			args:        []string{"-w", "2", "-t", "5"},
			wantWorkers: 2,
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "environment fills in only the unset flag",
			env:         map[string]string{"STRESSY_WORKERS": "8"},
			args:        []string{"-t", "5"},
			wantWorkers: 8,
			wantTimeout: 5 * time.Second,
		},
		{
			name:        "duration flag",
			args:        []string{"--timeout", "5m"},
			wantWorkers: 1,
			wantTimeout: 5 * time.Minute,
		},
		{
			name:        "compound duration flag",
			args:        []string{"-t", "1h30m"},
			wantWorkers: 1,
			wantTimeout: 90 * time.Minute,
		},
		// #26: STRESSY_TIMEOUT=60s is the natural thing to type, and before
		// --timeout took a duration it was the sharpest edge of #10 — the value
		// failed to parse, and the run that was meant to last a minute ran
		// forever and exited 0.
		{
			name:        "duration environment variable",
			env:         map[string]string{"STRESSY_TIMEOUT": "60s"},
			wantWorkers: 1,
			wantTimeout: 60 * time.Second,
		},
		{
			name:        "sub-second duration",
			args:        []string{"-t", "250ms"},
			wantWorkers: 1,
			wantTimeout: 250 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}

			if cfg.Workers != tt.wantWorkers {
				t.Errorf("Workers = %d, want %d", cfg.Workers, tt.wantWorkers)
			}
			if cfg.Timeout != tt.wantTimeout {
				t.Errorf("Timeout = %s, want %s", cfg.Timeout, tt.wantTimeout)
			}
		})
	}
}

// TestMalformedEnvValueIsRejected covers issue #10: a non-numeric
// STRESSY_TIMEOUT used to be swallowed, leaving the flag at pflag's zero value
// and turning a run the operator had bounded into an endless one, exit code 0.
func TestMalformedEnvValueIsRejected(t *testing.T) {
	t.Setenv("STRESSY_TIMEOUT", "not-a-number")

	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)
	cmd.SetArgs(nil)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want the malformed value to be rejected")
	}

	// The message has to name the offending variable and value, or an operator
	// staring at a failed run has nothing to go on.
	if !strings.Contains(err.Error(), "STRESSY_TIMEOUT") || !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("Execute() error = %q, want it to name the variable and the bad value", err)
	}
}

// TestMalformedFlagValueIsRejected is the command-line counterpart: pflag
// parses these itself, so this path reported the error even before #10.
func TestMalformedFlagValueIsRejected(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)
	cmd.SetArgs([]string{"-t", "not-a-number"})

	if err := cmd.Execute(); err == nil {
		t.Error("Execute() error = nil, want a parse error")
	}
}

// TestTimeoutHelpAdvertisesDuration guards the operator-facing half of #26: a
// timeout that accepts "5m" but still describes itself as an int of seconds
// leaves the reader no reason to try the duration form.
func TestTimeoutHelpAdvertisesDuration(t *testing.T) {
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatal(`Lookup("timeout") = nil, want the registered flag`)
	}

	if f.Value.Type() != "duration" {
		t.Errorf("timeout type = %q, want %q", f.Value.Type(), "duration")
	}
	if !strings.Contains(f.Usage, "5m") {
		t.Errorf("timeout usage = %q, want it to show the duration form", f.Usage)
	}
	if !strings.Contains(f.Usage, "seconds") {
		t.Errorf("timeout usage = %q, want it to state that a bare number is seconds", f.Usage)
	}
}
