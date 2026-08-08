package cli

import (
	"fmt"
	"math"
	"strconv"
	"time"
)

// durationValue adapts time.Duration to the pflag.Value interface. A bare number
// has meant seconds since 0.1.0 and time.ParseDuration rejects exactly that
// input, so this accepts both spellings and the two cannot collide.
type durationValue time.Duration

// newDurationValue writes the default through p, as pflag's own *VarP helpers do.
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

// Type is the placeholder pflag prints in help text, as in `--timeout duration`.
func (d *durationValue) Type() string { return "duration" }

func (d *durationValue) String() string { return time.Duration(*d).String() }

// parseDuration accepts both Go's duration syntax ("30s", "5m", "1h30m") and a
// bare count of seconds ("300"), which is the pre-0.4 --timeout spelling.
func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: want a duration such as 30s or 5m, or a bare number of seconds", s)
	}

	// time.ParseDuration reports its own overflow; this multiplication is ours.
	// Past ~292 years int64 nanoseconds wrap, and a wrapped value is negative or
	// a plausible short duration — both silently run for the wrong length of time.
	d := time.Duration(n) * time.Second
	if d/time.Second != time.Duration(n) {
		return 0, fmt.Errorf("invalid duration %q: %d seconds is out of range, the maximum is %d", s, n, math.MaxInt64/int64(time.Second))
	}

	return d, nil
}
