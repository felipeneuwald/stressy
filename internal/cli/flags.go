package cli

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// durationValue adapts time.Duration to the flag.Value interface. A bare number
// has meant seconds since 0.1.0 and time.ParseDuration rejects exactly that
// input, so this accepts both spellings and the two cannot collide.
type durationValue time.Duration

// newDurationValue writes the default through p, as flag's own DurationVar does.
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

// Type is the placeholder the Flags block prints, as in `--timeout duration`.
// The flag package has no use for it and ignores the extra method.
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

// workersValue adapts the worker count to the flag.Value interface. Stock
// IntVar reports `strconv.ParseInt: parsing "abc"` at an operator who may not
// write Go; named after the flag, because the message it produces names it.
type workersValue int

// newWorkersValue writes the default through p, as flag's own IntVar does.
func newWorkersValue(val int, p *int) *workersValue {
	*p = val

	return (*workersValue)(p)
}

func (w *workersValue) Set(s string) error {
	v, err := parseWorkers(s)
	if err != nil {
		return err
	}

	*w = workersValue(v)

	return nil
}

// Type is the placeholder the Flags block prints, as in `-w, --workers int`.
// The flag package has no use for it and ignores the extra method.
func (w *workersValue) Type() string { return "int" }

// String is what the flag package prints for a set flag, and what it records as
// DefValue at registration — which is where the `(default N)` in the help line
// comes from, captured before a command line can overwrite the value.
func (w *workersValue) String() string { return strconv.Itoa(int(*w)) }

// parseWorkers parses a worker count. The range is deliberately not checked
// here — a value the parser rejects is a usage error and gets the flag list
// with it — so 0 and -1 parse, and validateRanges rejects them a moment later
// as the runtime errors they are (#17a).
func parseWorkers(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err == nil {
		return n, nil
	}

	// "Not a number at all" and "a number no int can hold" want different words.
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("invalid workers %q: out of range, want a whole number from 1 to %d", s, math.MaxInt)
	}

	return 0, fmt.Errorf("invalid workers %q: want a whole number, 1 or greater", s)
}

// boolValue is what --help and --version are registered as. flag.BoolVar cannot
// be used for either: both spellings of a flag have to write through one Value,
// and BoolVar makes a new one per name.
type boolValue bool

// newBoolValue leaves p as it is; both flags default to false, and false is the
// zero value of the field each writes through.
func newBoolValue(p *bool) *boolValue { return (*boolValue)(p) }

func (b *boolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("invalid boolean %q: want true or false", s)
	}

	*b = boolValue(v)

	return nil
}

// IsBoolFlag is how the flag package knows `-h` takes no value of its own. Both
// flags would want one without it, and `stressy -h` would consume what follows.
func (b *boolValue) IsBoolFlag() bool { return true }

func (b *boolValue) String() string { return strconv.FormatBool(bool(*b)) }
