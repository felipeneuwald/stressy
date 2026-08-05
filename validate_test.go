package main

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// TestOutOfRangeValueNamesItsSource covers #51 through the command main() runs.
// A value that parsed and is out of range used to fail with the library's bare
// message however it arrived, so `STRESSY_WORKERS=0` and `-w 0` produced
// byte-identical output — while a value the same variable could not parse said
// `invalid STRESSY_WORKERS: …`. Two error paths for one variable, disagreeing
// about whether to name it, and in a compose file or a pod spec filling the
// variable by indirection the operator had no pointer to which layer produced
// the value.
func TestOutOfRangeValueNamesItsSource(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		// want is the whole message, not a fragment: the shape is the point of
		// #51, and a Contains check would pass on the bare message these used
		// to produce.
		want string
	}{
		{
			name: "workers from the environment",
			env:  map[string]string{"STRESSY_WORKERS": "0"},
			args: []string{"-t", "100ms"},
			want: "invalid STRESSY_WORKERS: workers must be 1 or greater",
		},
		{
			name: "timeout from the environment",
			env:  map[string]string{"STRESSY_TIMEOUT": "-5s"},
			args: []string{"-w", "1"},
			want: "invalid STRESSY_TIMEOUT: timeout must be 0 (indefinite) or greater",
		},
		// The command line names its own source by having been typed, so these
		// two keep the message they have always had — which `stressy -w 0`
		// exits 1 with, and TestExitCodes reads off a real process.
		{
			name: "workers from the command line",
			args: []string{"-w", "0"},
			want: "workers must be 1 or greater",
		},
		{
			name: "timeout from the command line",
			args: []string{"-w", "1", "-t", "-5s"},
			want: "timeout must be 0 (indefinite) or greater",
		},
		// A flag given on the command line is never filled from the
		// environment, so the variable is not what produced the rejected value
		// and naming it would send the operator to the wrong layer.
		{
			name: "command line loses, environment names itself",
			env:  map[string]string{"STRESSY_WORKERS": "4"},
			args: []string{"-t", "-5s"},
			want: "timeout must be 0 (indefinite) or greater",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built: newTestCmd blanks the ambient
			// STRESSY_* variables, and a case's own values have to survive
			// that. See clearStressyEnv.
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

			// Rejected before anything starts, which is what makes this worth
			// checking here rather than in Run: no worker, no startup line.
			if ran {
				t.Errorf("Execute(%q) started the run, want the configuration rejected first", tt.args)
			}
		})
	}
}

// TestOutOfRangeEnvironmentValueMatchesTheParseError is the symmetry #51 asks
// for, stated directly: the two ways STRESSY_WORKERS can be wrong have to agree
// about naming the variable. Only one of them did.
func TestOutOfRangeEnvironmentValueMatchesTheParseError(t *testing.T) {
	for _, value := range []string{"abc", "0"} {
		t.Run(value, func(t *testing.T) {
			var cfg stressy.Cfg
			cmd := newTestCmd(t, &cfg)

			// After the command is built; see clearStressyEnv.
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

// TestValidateRangesAcceptsWhatARunAccepts is the other direction: the check
// added in front of the run must not reject a configuration the run itself
// would have taken. The rules are internal/stressy's; this only attributes
// them.
func TestValidateRangesAcceptsWhatARunAccepts(t *testing.T) {
	tests := []struct {
		name string
		cfg  stressy.Cfg
	}{
		{name: "one worker, indefinite", cfg: stressy.Cfg{Workers: 1}},
		{name: "many workers, bounded", cfg: stressy.Cfg{Workers: 64, Timeout: time.Minute}},
		{name: "sub-second timeout", cfg: stressy.Cfg{Workers: 1, Timeout: 250 * time.Millisecond}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Every flag reported as environment-filled, which is the case that
			// would wrap a message if there were one to wrap.
			fromEnv := map[string]string{"workers": "STRESSY_WORKERS", "timeout": "STRESSY_TIMEOUT"}

			if err := validateRanges(&tt.cfg, fromEnv); err != nil {
				t.Errorf("validateRanges(%+v) error = %v, want nil", tt.cfg, err)
			}
		})
	}
}
