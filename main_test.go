package main

import (
	"io"
	"strings"
	"testing"

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
		wantTimeout int
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
			wantTimeout: 30,
		},
		{
			name:        "short flags",
			args:        []string{"-w", "4", "-t", "30"},
			wantWorkers: 4,
			wantTimeout: 30,
		},
		{
			name:        "environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			wantWorkers: 8,
			wantTimeout: 60,
		},
		{
			name:        "flags take precedence over environment variables",
			env:         map[string]string{"STRESSY_WORKERS": "8", "STRESSY_TIMEOUT": "60"},
			args:        []string{"-w", "2", "-t", "5"},
			wantWorkers: 2,
			wantTimeout: 5,
		},
		{
			name:        "environment fills in only the unset flag",
			env:         map[string]string{"STRESSY_WORKERS": "8"},
			args:        []string{"-t", "5"},
			wantWorkers: 8,
			wantTimeout: 5,
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
				t.Errorf("Timeout = %d, want %d", cfg.Timeout, tt.wantTimeout)
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

	// The message has to name the offending value, or an operator staring at a
	// failed run has nothing to go on.
	if !strings.Contains(err.Error(), "timeout") || !strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("Execute() error = %q, want it to name the flag and the bad value", err)
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
