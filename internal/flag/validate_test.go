package flag

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newCmdWithFlags builds a command with the given flag definitions registered.
func newCmdWithFlags(t *testing.T, flags []interface{}) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	if err := Load(cmd, flags); err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	return cmd
}

func TestValidateString(t *testing.T) {
	tests := []struct {
		name          string
		allowedValues []string
		set           string
		wantErr       bool
	}{
		{name: "default value is allowed", allowedValues: []string{"json", "yaml"}, wantErr: false},
		{name: "set to an allowed value", allowedValues: []string{"json", "yaml"}, set: "yaml", wantErr: false},
		{name: "set to a disallowed value", allowedValues: []string{"json", "yaml"}, set: "xml", wantErr: true},
		{name: "no allowed values skips validation", allowedValues: nil, set: "anything", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var format string
			flags := []interface{}{
				String{
					Pointer:          &format,
					FlagName:         "format",
					FlagDefaultValue: "json",
					FlagUsage:        "output format",
					AllowedValues:    tt.allowedValues,
				},
			}

			cmd := newCmdWithFlags(t, flags)
			if tt.set != "" {
				if err := cmd.Flags().Set("format", tt.set); err != nil {
					t.Fatalf("Set() error = %v, want nil", err)
				}
			}

			err := Validate(cmd, flags)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				if !strings.Contains(err.Error(), `invalid value "xml" for --format`) {
					t.Errorf("Validate() error = %q, want it to name the value and the flag", err)
				}
				if !strings.Contains(err.Error(), `"json", "yaml"`) {
					t.Errorf("Validate() error = %q, want it to list the allowed values", err)
				}
			}
		})
	}
}

func TestValidateInt(t *testing.T) {
	tests := []struct {
		name          string
		allowedValues []int
		set           string
		wantErr       bool
	}{
		{name: "default value is allowed", allowedValues: []int{1, 2, 4}, wantErr: false},
		{name: "set to an allowed value", allowedValues: []int{1, 2, 4}, set: "4", wantErr: false},
		{name: "set to a disallowed value", allowedValues: []int{1, 2, 4}, set: "3", wantErr: true},
		{name: "no allowed values skips validation", allowedValues: nil, set: "99", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var workers int
			flags := []interface{}{
				Int{
					Pointer:          &workers,
					FlagName:         "workers",
					FlagDefaultValue: 1,
					FlagUsage:        "number of workers",
					AllowedValues:    tt.allowedValues,
				},
			}

			cmd := newCmdWithFlags(t, flags)
			if tt.set != "" {
				if err := cmd.Flags().Set("workers", tt.set); err != nil {
					t.Fatalf("Set() error = %v, want nil", err)
				}
			}

			err := Validate(cmd, flags)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && !strings.Contains(err.Error(), `invalid value "3" for --workers`) {
				t.Errorf("Validate() error = %q, want it to name the value and the flag", err)
			}
		})
	}
}

func TestValidateUnsupportedType(t *testing.T) {
	var workers int
	cmd := newCmdWithFlags(t, []interface{}{
		Int{Pointer: &workers, FlagName: "workers", FlagDefaultValue: 1, FlagUsage: "number of workers"},
	})

	// Validate walks the registered flags and inspects every definition it is
	// handed, so an unsupported definition is reported even though the command
	// itself only carries supported flags.
	err := Validate(cmd, []interface{}{"not-a-flag-definition"})
	if err == nil {
		t.Fatal("Validate() error = nil, want an unsupported type error")
	}

	if !strings.Contains(err.Error(), "can't validate unsupported flag type: string") {
		t.Errorf("Validate() error = %q, want it to name the unsupported type", err)
	}
}
