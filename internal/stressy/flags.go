package stressy

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// durationValue adapts time.Duration to the flag.Value interface. Stock
// DurationVar rejects everything time.ParseDuration does not take as a bare
// "parse error", which tells an operator nothing about what a good value looks
// like.
//
// What Set returns is the guidance and nothing else, because the flag package
// wraps it in `invalid value %q for flag -%s:` before an operator ever reads
// it: naming the value here as well put it, and the flag, in the line twice
// (#123).
type durationValue time.Duration

// newDurationValue writes the default through p, as flag's own DurationVar does.
func newDurationValue(val time.Duration, p *time.Duration) *durationValue {
	*p = val

	return (*durationValue)(p)
}

// Set takes Go's duration syntax and nothing else. A bare count of seconds was
// accepted from 0.1.0 to 0.5.0 and is now rejected, loudly: `-t 60` fails here
// rather than quietly running for a length nobody typed.
func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return errors.New("want a duration such as 30s or 5m")
	}

	*d = durationValue(v)

	return nil
}

// Type is the placeholder the Flags block prints, as in `--timeout duration`.
// The flag package has no use for it and ignores the extra method.
func (d *durationValue) Type() string { return "duration" }

func (d *durationValue) String() string { return time.Duration(*d).String() }

// workersValue adapts the worker count to the flag.Value interface. Stock
// IntVar reports `strconv.ParseInt: parsing "abc"` at an operator who may not
// write Go. Its message is the guidance alone, for durationValue's reason.
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
// with it — so 0 and -1 parse, and Cfg.validate rejects them a moment later as
// the runtime errors they are (#17a).
//
// Nothing calls it but workersValue.Set, so its message is only ever read
// through the flag package's wrapper, and names neither the value nor the flag.
func parseWorkers(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err == nil {
		return n, nil
	}

	// "Not a number at all" and "a number no int can hold" want different words.
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("out of range, want a whole number from 1 to %d", math.MaxInt)
	}

	return 0, errors.New("want a whole number, 1 or greater")
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
		return errors.New("want true or false")
	}

	*b = boolValue(v)

	return nil
}

// IsBoolFlag is how the flag package knows `-h` takes no value of its own. Both
// flags would want one without it, and `stressy -h` would consume what follows.
func (b *boolValue) IsBoolFlag() bool { return true }

func (b *boolValue) String() string { return strconv.FormatBool(bool(*b)) }
