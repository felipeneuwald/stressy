package flag

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLoad(t *testing.T) {
	var (
		format  string
		workers int
	)

	cmd := &cobra.Command{Use: "test"}
	err := Load(cmd, []interface{}{
		String{
			Pointer:          &format,
			FlagName:         "format",
			FlagShortHand:    "f",
			FlagDefaultValue: "json",
			FlagUsage:        "output format",
			AllowedValues:    []string{"json", "yaml"},
		},
		Int{
			Pointer:          &workers,
			FlagName:         "workers",
			FlagDefaultValue: 1,
			FlagUsage:        "number of workers",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("Lookup(\"format\") = nil, want the registered flag")
	}
	if formatFlag.Shorthand != "f" {
		t.Errorf("format shorthand = %q, want %q", formatFlag.Shorthand, "f")
	}
	if formatFlag.DefValue != "json" {
		t.Errorf("format default = %q, want %q", formatFlag.DefValue, "json")
	}
	if want := `output format (allowed: "json", "yaml")`; formatFlag.Usage != want {
		t.Errorf("format usage = %q, want %q", formatFlag.Usage, want)
	}
	if format != "json" {
		t.Errorf("format = %q, want the default %q written through the pointer", format, "json")
	}

	workersFlag := cmd.Flags().Lookup("workers")
	if workersFlag == nil {
		t.Fatal("Lookup(\"workers\") = nil, want the registered flag")
	}
	if workersFlag.Shorthand != "" {
		t.Errorf("workers shorthand = %q, want none", workersFlag.Shorthand)
	}
	// No AllowedValues, so the usage text is left untouched.
	if want := "number of workers"; workersFlag.Usage != want {
		t.Errorf("workers usage = %q, want %q", workersFlag.Usage, want)
	}
	if workers != 1 {
		t.Errorf("workers = %d, want the default 1 written through the pointer", workers)
	}
}

func TestLoadIntAllowedValuesInUsage(t *testing.T) {
	var workers int

	cmd := &cobra.Command{Use: "test"}
	err := Load(cmd, []interface{}{
		Int{
			Pointer:          &workers,
			FlagName:         "workers",
			FlagShortHand:    "w",
			FlagDefaultValue: 1,
			FlagUsage:        "number of workers",
			AllowedValues:    []int{1, 2, 4},
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if want := `number of workers (allowed: "1", "2", "4")`; cmd.Flags().Lookup("workers").Usage != want {
		t.Errorf("workers usage = %q, want %q", cmd.Flags().Lookup("workers").Usage, want)
	}
}

func TestLoadUnsupportedType(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	err := Load(cmd, []interface{}{"not-a-flag-definition"})
	if err == nil {
		t.Fatal("Load() error = nil, want an unsupported type error")
	}

	if !strings.Contains(err.Error(), "can't load unsupported flag type: string") {
		t.Errorf("Load() error = %q, want it to name the unsupported type", err)
	}
}
