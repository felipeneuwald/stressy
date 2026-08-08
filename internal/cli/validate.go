package cli

import (
	"fmt"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// validateRanges rejects a value that parsed but is out of range, naming the
// environment variable it arrived through when it did. This is the last point at
// which that source is known — by the time Run holds the value, `-w 0`,
// STRESSY_WORKERS=0 and the default are one int — and internal/stressy re-applies
// the same rules there for callers that do not come through this command.
func validateRanges(cfg *stressy.Cfg, fromEnv map[string]string) error {
	// A setting missing here is unattributed, not unchecked. See validateConfig.
	checks := []struct {
		flag string
		err  error
	}{
		{flag: "workers", err: stressy.ValidateWorkers(cfg.Workers)},
		{flag: "timeout", err: stressy.ValidateTimeout(cfg.Timeout)},
		{flag: "report", err: stressy.ValidateReport(cfg.Report)},
	}

	for _, check := range checks {
		if check.err == nil {
			continue
		}

		// Deliberately the shape bindEnv gives a value it could not parse.
		if name, ok := fromEnv[check.flag]; ok {
			return fmt.Errorf("invalid %v: %w", name, check.err)
		}

		return check.err
	}

	return nil
}
