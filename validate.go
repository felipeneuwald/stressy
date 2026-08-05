package main

import (
	"fmt"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// validateRanges rejects a value that parsed but is out of range, naming the
// environment variable it arrived through when it arrived through one.
//
// The rules belong to internal/stressy, which applies them again in Run and
// stays the backstop for any caller that does not come through this command.
// What that package cannot know is where a value came from: by the time Run
// holds it, `-w 0`, STRESSY_WORKERS=0 and the default are one int. So
// STRESSY_WORKERS=0 failed with a bare `workers must be 1 or greater`,
// byte-identical to the `-w 0` case, while a value the same variable could not
// even parse said `invalid STRESSY_WORKERS: …`. Two error paths for one
// variable, disagreeing about whether to name it — and in a compose file or a
// pod spec where the value arrives through `STRESSY_WORKERS=${WORKERS}`
// indirection, which is a shape bindEnv keeps working deliberately, the
// operator is left with no pointer to which setting in which layer produced it
// (#51).
//
// Running from PreRunE rather than from RunE also keeps the failure on the
// silent side of #17a: SilenceUsage is already set by the time this is called,
// so a rejected value gets its one line instead of the whole flag list.
//
// A value given on the command line keeps the message it has always had. `-w 0`
// names its own source by having been typed, and pflag's `-w, --workers`
// wrapper belongs to the parse path rather than to this one.
func validateRanges(cfg *stressy.Cfg, fromEnv map[string]string) error {
	// One entry per flag newCmd registers. A setting missing from this list is
	// not unchecked, only unattributed: validateConfig applies the same rules
	// to whatever reaches Run, before a single worker starts.
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

		// Deliberately the shape bindEnv gives a value the same variable failed
		// to parse. Matching it is the point of #51.
		if name, ok := fromEnv[check.flag]; ok {
			return fmt.Errorf("invalid %v: %w", name, check.err)
		}

		return check.err
	}

	return nil
}
