package main

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// durationValue adapts time.Duration to the pflag.Value interface.
//
// pflag already ships DurationVarP, but its parser is time.ParseDuration, which
// rejects a unit-less number. --timeout has meant seconds since 0.1.0, so the
// stock type would turn every existing `stressy -t 300` and STRESSY_TIMEOUT=300
// into a parse error. This accepts both spellings instead: the two cannot
// collide, because a bare integer is exactly the input time.ParseDuration
// refuses.
type durationValue time.Duration

// newDurationValue writes the default through the caller's pointer, the same
// way pflag's own *VarP helpers do, and returns a pflag.Value aliasing it.
func newDurationValue(val time.Duration, p *time.Duration) *durationValue {
	*p = val
	return (*durationValue)(p)
}

func (d *durationValue) Set(s string) error {
	v, err := parseDuration(s)
	if err != nil {
		return err
	}

	*d = durationValue(v)

	return nil
}

// Type is the value placeholder pflag prints in help text, as in
// `-t, --timeout duration`.
func (d *durationValue) Type() string { return "duration" }

func (d *durationValue) String() string { return time.Duration(*d).String() }

// parseDuration parses a duration, accepting both Go's duration syntax ("30s",
// "5m", "1h30m") and a bare count of seconds ("300").
//
// The bare form is the pre-0.4 --timeout syntax, kept working so existing
// command lines and STRESSY_TIMEOUT values keep the meaning they have today
// rather than becoming errors.
func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: want a duration such as 30s or 5m, or a bare number of seconds", s)
	}

	// time.ParseDuration reports its own overflow, but this multiplication is
	// ours: at int64 nanoseconds anything past ~292 years wraps, and a wrapped
	// value is either negative or a plausible-looking short duration. Both are
	// worse than an error, because both silently run for the wrong length of
	// time.
	d := time.Duration(n) * time.Second
	if d/time.Second != time.Duration(n) {
		return 0, fmt.Errorf("invalid duration %q: %d seconds is out of range, the maximum is %d", s, n, math.MaxInt64/int64(time.Second))
	}

	return d, nil
}
