package flag

import (
	"os"
	"testing"

	"github.com/felipeneuwald/stressy/internal/ptr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestBind(t *testing.T) {
	tests := []struct {
		name            string
		setupFlags      func(*cobra.Command)
		setupViper      func(*viper.Viper)
		setCommandFlags func(*cobra.Command)
		validate        func(*testing.T, *cobra.Command)
		wantErr         bool
	}{
		{
			name: "string flag from viper",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("format", "json", "output format")
			},
			setupViper: func(v *viper.Viper) {
				v.Set("format", "yaml")
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetString("format")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != "yaml" {
					t.Errorf("flag value = %q, want %q", val, "yaml")
				}
			},
		},
		{
			name: "int flag from viper",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Int("threads", 1, "number of threads")
			},
			setupViper: func(v *viper.Viper) {
				v.Set("threads", 4)
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetInt("threads")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != 4 {
					t.Errorf("flag value = %d, want %d", val, 4)
				}
			},
		},
		{
			name: "bool flag from viper",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Bool("verbose", false, "verbose output")
			},
			setupViper: func(v *viper.Viper) {
				v.Set("verbose", true)
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetBool("verbose")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if !val {
					t.Error("flag value = false, want true")
				}
			},
		},
		{
			name: "command line flag takes precedence",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("format", "json", "output format")
			},
			setupViper: func(v *viper.Viper) {
				v.Set("format", "yaml")
			},
			setCommandFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "text")
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetString("format")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != "text" {
					t.Errorf("flag value = %q, want %q", val, "text")
				}
			},
		},
		{
			name: "viper value not set",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("format", "json", "output format")
			},
			setupViper: func(v *viper.Viper) {
				// Don't set any value in viper
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetString("format")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != "json" {
					t.Errorf("flag value = %q, want %q", val, "json")
				}
			},
		},
		{
			name: "multiple flags mixed sources",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("format", "json", "output format")
				cmd.Flags().Int("threads", 1, "number of threads")
				cmd.Flags().Bool("verbose", false, "verbose output")
			},
			setupViper: func(v *viper.Viper) {
				v.Set("format", "yaml")
				v.Set("threads", 4)
				v.Set("verbose", true)
			},
			setCommandFlags: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "text") // Override viper value
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				format, _ := cmd.Flags().GetString("format")
				threads, _ := cmd.Flags().GetInt("threads")
				verbose, _ := cmd.Flags().GetBool("verbose")

				if format != "text" {
					t.Errorf("format = %q, want %q", format, "text")
				}
				if threads != 4 {
					t.Errorf("threads = %d, want %d", threads, 4)
				}
				if !verbose {
					t.Error("verbose = false, want true")
				}
			},
		},
		{
			name: "type conversion from viper",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().Int("count", 1, "count value")
			},
			setupViper: func(v *viper.Viper) {
				// Viper might have the value as float64 from JSON
				v.Set("count", float64(42))
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetInt("count")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != 42 {
					t.Errorf("flag value = %d, want %d", val, 42)
				}
			},
		},
		{
			name: "environment variable via viper",
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("config", "", "config path")
			},
			setupViper: func(v *viper.Viper) {
				os.Setenv("STRESSY_CONFIG", "/etc/stressy/config.yaml")
				v.SetEnvPrefix("STRESSY")
				v.AutomaticEnv()
			},
			validate: func(t *testing.T, cmd *cobra.Command) {
				val, err := cmd.Flags().GetString("config")
				if err != nil {
					t.Errorf("failed to get flag value: %v", err)
				}
				if val != "/etc/stressy/config.yaml" {
					t.Errorf("flag value = %q, want %q", val, "/etc/stressy/config.yaml")
				}
				os.Unsetenv("STRESSY_CONFIG")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			v := viper.New()

			// Setup flags and viper
			if tt.setupFlags != nil {
				tt.setupFlags(cmd)
			}
			if tt.setupViper != nil {
				tt.setupViper(v)
			}
			if tt.setCommandFlags != nil {
				tt.setCommandFlags(cmd)
			}

			// Run the bind function
			err := Bind(cmd, v)
			if (err != nil) != tt.wantErr {
				t.Errorf("Bind() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Validate results
			if err == nil && tt.validate != nil {
				tt.validate(t, cmd)
			}
		})
	}
}

func TestBind_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		setupCmd   func() *cobra.Command
		setupViper func() *viper.Viper
		wantErr    bool
		errMsg     string
	}{
		{
			name: "nil command",
			setupCmd: func() *cobra.Command {
				return nil
			},
			setupViper: func() *viper.Viper {
				return viper.New()
			},
			wantErr: true,
			errMsg:  "cmd cannot be nil",
		},
		{
			name: "nil viper",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			setupViper: func() *viper.Viper {
				return nil
			},
			wantErr: true,
			errMsg:  "viper instance cannot be nil",
		},
		{
			name: "command with no flags",
			setupCmd: func() *cobra.Command {
				return &cobra.Command{}
			},
			setupViper: func() *viper.Viper {
				return viper.New()
			},
			wantErr: false,
		},
		{
			name: "viper with invalid type",
			setupCmd: func() *cobra.Command {
				cmd := &cobra.Command{}
				cmd.Flags().Int("count", 0, "count value")
				return cmd
			},
			setupViper: func() *viper.Viper {
				v := viper.New()
				v.Set("count", "not a number")
				return v
			},
			wantErr: false, // Should handle the error gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setupCmd()
			v := tt.setupViper()

			err := Bind(cmd, v)
			if (err != nil) != tt.wantErr {
				t.Errorf("Bind() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && err.Error() != tt.errMsg {
				t.Errorf("Bind() error = %v, want error message %q", err, tt.errMsg)
			}
		})
	}
}

func TestBind_FlagPersistence(t *testing.T) {
	// Test that flag values persist correctly after binding
	cmd := &cobra.Command{}
	v := viper.New()

	// Setup initial flags
	strVal := ptr.StrPtr("json")
	intVal := ptr.IntPtr(1)
	boolVal := ptr.BoolPtr(false)

	cmd.Flags().StringVar(strVal, "format", "json", "output format")
	cmd.Flags().IntVar(intVal, "threads", 1, "number of threads")
	cmd.Flags().BoolVar(boolVal, "verbose", false, "verbose output")

	// Set values in viper
	v.Set("format", "yaml")
	v.Set("threads", 4)
	v.Set("verbose", true)

	// Bind values
	if err := Bind(cmd, v); err != nil {
		t.Fatalf("Bind() failed: %v", err)
	}

	// Verify that pointer values were updated
	if *strVal != "yaml" {
		t.Errorf("string pointer value = %q, want %q", *strVal, "yaml")
	}
	if *intVal != 4 {
		t.Errorf("int pointer value = %d, want %d", *intVal, 4)
	}
	if !*boolVal {
		t.Error("bool pointer value = false, want true")
	}

	// Modify values through pointers
	*strVal = "text"
	*intVal = 8
	*boolVal = false

	// Verify that flag values reflect the changes
	format, _ := cmd.Flags().GetString("format")
	threads, _ := cmd.Flags().GetInt("threads")
	verbose, _ := cmd.Flags().GetBool("verbose")

	if format != "text" {
		t.Errorf("flag format = %q, want %q", format, "text")
	}
	if threads != 8 {
		t.Errorf("flag threads = %d, want %d", threads, 8)
	}
	if verbose {
		t.Error("flag verbose = true, want false")
	}
}
