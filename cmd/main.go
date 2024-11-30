package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/felipeneuwald/stressy/internal/flag"
	"github.com/felipeneuwald/stressy/internal/stressy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "0.0.0"
	c       stressy.Cfg
	cmd     = &cobra.Command{
		Use:   "stressy",
		Short: "Stressy is a simple tool to perform CPU stress tests",
		Long: `Stressy is a simple tool to perform CPU stress tests.

All flags can be configured using environment variables with the STRESSY_ prefix. 
For example: STRESSY_WORKERS=4 or STRESSY_TIMEOUT=60.`,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Version:           version,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			v := viper.New()
			v.SetEnvPrefix("STRESSY")
			v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			v.AutomaticEnv()
			flag.Bind(cmd, v)
			if err := flag.Validate(cmd, cobraFlags); err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s := stressy.New(c)
			return s.Run()
		},
	}
	cobraFlags = []interface{}{
		flag.Int{
			Pointer:          &c.Workers,
			FlagName:         "workers",
			FlagShortHand:    "w",
			FlagDefaultValue: 1,
			FlagUsage:        "number of parallel workers for CPU stress testing",
		},
		flag.Int{
			Pointer:          &c.Timeout,
			FlagName:         "timeout",
			FlagShortHand:    "t",
			FlagDefaultValue: 0,
			FlagUsage:        "timeout in seconds for the CPU stress test",
		},
	}
)

func main() {
	if err := flag.Load(cmd, cobraFlags); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading flags: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
