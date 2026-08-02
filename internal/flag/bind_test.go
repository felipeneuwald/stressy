package flag

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestBindNilArguments(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	if err := Bind(nil, viper.New()); err == nil {
		t.Error("Bind(nil, v) error = nil, want an error")
	}

	if err := Bind(cmd, nil); err == nil {
		t.Error("Bind(cmd, nil) error = nil, want an error")
	}
}

func TestBind(t *testing.T) {
	tests := []struct {
		name        string
		viperValue  any // nil means the key is not set in viper
		commandLine string
		want        int
	}{
		{name: "nothing set keeps the default", want: 1},
		{name: "viper value is applied", viperValue: 8, want: 8},
		{name: "command line wins over viper", viperValue: 8, commandLine: "2", want: 2},
		{name: "command line is kept when viper is unset", commandLine: "2", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var workers int
			cmd := newCmdWithFlags(t, []interface{}{
				Int{Pointer: &workers, FlagName: "workers", FlagDefaultValue: 1, FlagUsage: "number of workers"},
			})

			// Setting through Flags() marks the flag as Changed, which is how
			// pflag records "the user passed this on the command line".
			if tt.commandLine != "" {
				if err := cmd.Flags().Set("workers", tt.commandLine); err != nil {
					t.Fatalf("Set() error = %v, want nil", err)
				}
			}

			v := viper.New()
			if tt.viperValue != nil {
				v.Set("workers", tt.viperValue)
			}

			if err := Bind(cmd, v); err != nil {
				t.Fatalf("Bind() error = %v, want nil", err)
			}

			if workers != tt.want {
				t.Errorf("workers = %d, want %d", workers, tt.want)
			}
		})
	}
}
