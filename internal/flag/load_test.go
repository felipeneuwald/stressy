package flag

import (
	"strings"
	"testing"

	"github.com/felipeneuwald/stressy/internal/ptr"
	"github.com/spf13/cobra"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name          string
		flags         []interface{}
		validateFlags func(*testing.T, *cobra.Command)
		wantErr       bool
		wantErrMatch  string
	}{
		{
			name: "string flag without allowed values",
			flags: []interface{}{
				String{
					FlagName:         "format",
					FlagUsage:        "output format",
					FlagDefaultValue: "json",
					Pointer:          ptr.StrPtr("json"),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				flag := cmd.Flags().Lookup("format")
				if flag == nil {
					t.Error("flag 'format' not found")
					return
				}
				if flag.Usage != "output format" {
					t.Errorf("unexpected usage: got %q, want %q", flag.Usage, "output format")
				}
			},
		},
		{
			name: "string flag with allowed values",
			flags: []interface{}{
				String{
					FlagName:         "format",
					FlagUsage:        "output format",
					FlagDefaultValue: "json",
					AllowedValues:    []string{"json", "yaml", "text"},
					Pointer:          ptr.StrPtr("json"),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				flag := cmd.Flags().Lookup("format")
				if flag == nil {
					t.Error("flag 'format' not found")
					return
				}
				expectedUsage := `output format (allowed: "json", "yaml", "text")`
				if flag.Usage != expectedUsage {
					t.Errorf("unexpected usage: got %q, want %q", flag.Usage, expectedUsage)
				}
			},
		},
		{
			name: "int flag without allowed values",
			flags: []interface{}{
				Int{
					FlagName:         "threads",
					FlagUsage:        "number of threads",
					FlagDefaultValue: 4,
					Pointer:          ptr.IntPtr(4),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				flag := cmd.Flags().Lookup("threads")
				if flag == nil {
					t.Error("flag 'threads' not found")
					return
				}
				if flag.Usage != "number of threads" {
					t.Errorf("unexpected usage: got %q, want %q", flag.Usage, "number of threads")
				}
			},
		},
		{
			name: "int flag with allowed values",
			flags: []interface{}{
				Int{
					FlagName:         "threads",
					FlagUsage:        "number of threads",
					FlagDefaultValue: 4,
					AllowedValues:    []int{2, 4, 8},
					Pointer:          ptr.IntPtr(4),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				flag := cmd.Flags().Lookup("threads")
				if flag == nil {
					t.Error("flag 'threads' not found")
					return
				}
				expectedUsage := `number of threads (allowed: "2", "4", "8")`
				if flag.Usage != expectedUsage {
					t.Errorf("unexpected usage: got %q, want %q", flag.Usage, expectedUsage)
				}
			},
		},
		{
			name: "flag with shorthand",
			flags: []interface{}{
				String{
					FlagName:         "format",
					FlagShortHand:    "f",
					FlagUsage:        "output format",
					FlagDefaultValue: "json",
					Pointer:          ptr.StrPtr("json"),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				flag := cmd.Flags().ShorthandLookup("f")
				if flag == nil {
					t.Error("shorthand flag 'f' not found")
					return
				}
				if flag.Name != "format" {
					t.Errorf("unexpected flag name: got %q, want %q", flag.Name, "format")
				}
			},
		},
		{
			name: "multiple flags",
			flags: []interface{}{
				String{
					FlagName:         "format",
					FlagUsage:        "output format",
					FlagDefaultValue: "json",
					AllowedValues:    []string{"json", "yaml"},
					Pointer:          ptr.StrPtr("json"),
				},
				Int{
					FlagName:         "threads",
					FlagUsage:        "number of threads",
					FlagDefaultValue: 4,
					AllowedValues:    []int{2, 4, 8},
					Pointer:          ptr.IntPtr(4),
				},
			},
			validateFlags: func(t *testing.T, cmd *cobra.Command) {
				formatFlag := cmd.Flags().Lookup("format")
				threadsFlag := cmd.Flags().Lookup("threads")
				if formatFlag == nil || threadsFlag == nil {
					t.Error("one or more flags not found")
					return
				}
			},
		},
		{
			name: "unsupported flag type",
			flags: []interface{}{
				struct{ FlagName string }{"dummy"},
			},
			wantErr:      true,
			wantErrMatch: "can't load unsupported flag type",
		},
		{
			name:    "nil flags slice",
			flags:   nil,
			wantErr: false,
		},
		{
			name:    "empty flags slice",
			flags:   []interface{}{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			err := Load(cmd, tt.flags)

			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
				t.Errorf("Load() error = %v, want error containing %q", err, tt.wantErrMatch)
				return
			}

			if !tt.wantErr && tt.validateFlags != nil {
				tt.validateFlags(t, cmd)
			}
		})
	}
}

func TestLoad_FlagModification(t *testing.T) {
	// Test that modifying flag values after loading works correctly
	cmd := &cobra.Command{}
	strValue := ptr.StrPtr("json")
	
	flags := []interface{}{
		String{
			FlagName:         "format",
			FlagUsage:        "output format",
			FlagDefaultValue: "json",
			AllowedValues:    []string{"json", "yaml"},
			Pointer:          strValue,
		},
	}

	if err := Load(cmd, flags); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify initial value
	if *strValue != "json" {
		t.Errorf("initial value = %v, want json", *strValue)
	}

	// Modify flag value
	if err := cmd.Flags().Set("format", "yaml"); err != nil {
		t.Errorf("failed to set flag value: %v", err)
	}

	// Verify modified value
	if *strValue != "yaml" {
		t.Errorf("modified value = %v, want yaml", *strValue)
	}
}

func TestLoad_UsageFormatting(t *testing.T) {
	// Test that usage text is formatted correctly with various allowed values
	tests := []struct {
		name         string
		flag         interface{}
		wantUsage    string
	}{
		{
			name: "string flag with spaces in allowed values",
			flag: String{
				FlagName:         "mode",
				FlagUsage:        "operation mode",
				FlagDefaultValue: "normal mode",
				AllowedValues:    []string{"normal mode", "fast mode", "safe mode"},
				Pointer:          ptr.StrPtr("normal mode"),
			},
			wantUsage: `operation mode (allowed: "normal mode", "fast mode", "safe mode")`,
		},
		{
			name: "int flag with negative allowed values",
			flag: Int{
				FlagName:         "priority",
				FlagUsage:        "task priority",
				FlagDefaultValue: 0,
				AllowedValues:    []int{-1, 0, 1},
				Pointer:          ptr.IntPtr(0),
			},
			wantUsage: `task priority (allowed: "-1", "0", "1")`,
		},
		{
			name: "string flag with special characters",
			flag: String{
				FlagName:         "format",
				FlagUsage:        "output format",
				FlagDefaultValue: "json+comments",
				AllowedValues:    []string{"json+comments", "yaml+comments", "plain"},
				Pointer:          ptr.StrPtr("json+comments"),
			},
			wantUsage: `output format (allowed: "json+comments", "yaml+comments", "plain")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			if err := Load(cmd, []interface{}{tt.flag}); err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			var flagName string
			switch v := tt.flag.(type) {
			case String:
				flagName = v.FlagName
			case Int:
				flagName = v.FlagName
			}

			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Fatalf("flag %q not found", flagName)
			}

			if flag.Usage != tt.wantUsage {
				t.Errorf("usage = %q, want %q", flag.Usage, tt.wantUsage)
			}
		})
	}
}
