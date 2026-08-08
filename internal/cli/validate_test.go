package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// TestOutOfRangeValueNamesItsSource covers #51: `STRESSY_WORKERS=0` and `-w 0`
// failed byte-identically, where an unparseable value named the variable.
func TestOutOfRangeValueNamesItsSource(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		// want is the whole message, not a fragment: the shape is the point.
		want string
	}{
		{name: "workers from the environment", env: map[string]string{"STRESSY_WORKERS": "0"}, args: []string{"-t", "100ms"}, want: "invalid STRESSY_WORKERS: workers must be 1 or greater"},
		{name: "timeout from the environment", env: map[string]string{"STRESSY_TIMEOUT": "-5s"}, args: []string{"-w", "1"}, want: "invalid STRESSY_TIMEOUT: timeout must be 0 (indefinite) or greater"},
		{name: "report from the environment", env: map[string]string{"STRESSY_REPORT": "-30s"}, args: []string{"-w", "1"}, want: "invalid STRESSY_REPORT: report must be 0 (off) or greater"},
		{name: "workers from the command line", args: []string{"-w", "0"}, want: "workers must be 1 or greater"},
		{name: "timeout from the command line", args: []string{"-w", "1", "-t", "-5s"}, want: "timeout must be 0 (indefinite) or greater"},
		{name: "report from the command line", args: []string{"-w", "1", "-r", "-30s"}, want: "report must be 0 (off) or greater"},
		// A flag given on the command line is never filled from the environment.
		{name: "command line loses, environment names itself", env: map[string]string{"STRESSY_WORKERS": "4"}, args: []string{"-t", "-5s"}, want: "timeout must be 0 (indefinite) or greater"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			var ran bool
			cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute(%q) error = nil, want the value to be rejected", tt.args)
			}

			if err.Error() != tt.want {
				t.Errorf("Execute(%q) error = %q, want %q", tt.args, err, tt.want)
			}

			if ran {
				t.Errorf("Execute(%q) started the run, want the configuration rejected first", tt.args)
			}
		})
	}
}

// TestOutOfRangeEnvironmentValueMatchesTheParseError: both ways must name it (#51).
func TestOutOfRangeEnvironmentValueMatchesTheParseError(t *testing.T) {
	for _, value := range []string{"abc", "0"} {
		t.Run(value, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			t.Setenv("STRESSY_WORKERS", value)

			cmd.SetArgs([]string{"-t", "100ms"})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() with STRESSY_WORKERS=%s error = nil, want it rejected", value)
			}

			if !strings.HasPrefix(err.Error(), "invalid STRESSY_WORKERS: ") {
				t.Errorf("Execute() with STRESSY_WORKERS=%s error = %q, want it to open by naming the variable", value, err)
			}
		})
	}
}
