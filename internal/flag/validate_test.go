package flag

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name          string
		setupCmd      func() *cobra.Command
		flags         []interface{}
		wantErr      bool
		wantErrMatch string
	}{
		{
			name: "valid string flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("mode", "fast", "test flag")
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "mode",
					AllowedValues: []string{"fast", "slow"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid int flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().Int("threads", 4, "test flag")
				return cmd
			},
			flags: []interface{}{
				Int{
					FlagName:      "threads",
					AllowedValues: []int{2, 4, 8},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid string flag value",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("mode", "invalid", "test flag")
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "mode",
					AllowedValues: []string{"fast", "slow"},
				},
			},
			wantErr:      true,
			wantErrMatch: `invalid value "invalid" for --mode`,
		},
		{
			name: "invalid int flag value",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().Int("threads", 3, "test flag")
				return cmd
			},
			flags: []interface{}{
				Int{
					FlagName:      "threads",
					AllowedValues: []int{2, 4, 8},
				},
			},
			wantErr:      true,
			wantErrMatch: `invalid value "3" for --threads`,
		},
		{
			name: "unsupported flag type",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("dummy", "value", "test flag")
				return cmd
			},
			flags: []interface{}{
				struct{ FlagName string }{"dummy"}, // unsupported type
			},
			wantErr:      true,
			wantErrMatch: "can't validate unsupported flag type: struct { FlagName string }",
		},
		{
			name: "multiple valid flags",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("mode", "fast", "test flag")
				cmd.Flags().Int("threads", 4, "test flag")
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "mode",
					AllowedValues: []string{"fast", "slow"},
				},
				Int{
					FlagName:      "threads",
					AllowedValues: []int{2, 4, 8},
				},
			},
			wantErr: false,
		},
		{
			name: "flag without allowed values",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("mode", "anything", "test flag")
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "mode",
					AllowedValues: []string{}, // empty allowed values
				},
			},
			wantErr: false,
		},
		{
			name: "nil allowed values",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().String("mode", "anything", "test flag")
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "mode",
					AllowedValues: nil,
				},
			},
			wantErr: false,
		},
		{
			name: "empty flags slice",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			flags:    []interface{}{},
			wantErr: false,
		},
		{
			name: "nil flags slice",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			flags:    nil,
			wantErr: false,
		},
		{
			name: "non-existent flag",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				return cmd
			},
			flags: []interface{}{
				String{
					FlagName:      "non-existent",
					AllowedValues: []string{"value"},
				},
			},
			wantErr: false, // should not error as flag is not found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			err := Validate(cmd, tt.flags)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
				t.Errorf("Validate() error = %v, want error containing %v", err, tt.wantErrMatch)
			}
		})
	}
}

func TestValidate_FlagModification(t *testing.T) {
	// Test that modifying flag value after validation doesn't affect the result
	cmd := &cobra.Command{}
	cmd.Flags().String("mode", "fast", "test flag")
	
	flags := []interface{}{
		String{
			FlagName:      "mode",
			AllowedValues: []string{"fast", "slow"},
		},
	}

	// First validation should pass
	if err := Validate(cmd, flags); err != nil {
		t.Errorf("First validation failed: %v", err)
	}

	// Modify flag value to invalid
	cmd.Flags().Set("mode", "invalid")

	// Second validation should fail
	if err := Validate(cmd, flags); err == nil {
		t.Error("Second validation should have failed with invalid value")
	}
}

func TestValidate_ConcurrentFlags(t *testing.T) {
	// Test validation of multiple flags concurrently
	cmd := &cobra.Command{}
	cmd.Flags().String("mode", "fast", "test flag")
	cmd.Flags().Int("threads", 4, "test flag")
	cmd.Flags().String("format", "json", "test flag")

	flags := []interface{}{
		String{
			FlagName:      "mode",
			AllowedValues: []string{"fast", "slow"},
		},
		Int{
			FlagName:      "threads",
			AllowedValues: []int{2, 4, 8},
		},
		String{
			FlagName:      "format",
			AllowedValues: []string{"json", "xml"},
		},
	}

	// All flags are valid
	if err := Validate(cmd, flags); err != nil {
		t.Errorf("Validation failed with valid flags: %v", err)
	}

	// Make one flag invalid
	cmd.Flags().Set("format", "yaml")

	// Should fail due to invalid format
	if err := Validate(cmd, flags); err == nil {
		t.Error("Validation should have failed with invalid format")
	}
}
