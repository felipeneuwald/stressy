package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// workersValue adapts the worker count to the pflag.Value interface. Stock
// IntVarP reports `strconv.ParseInt: parsing "abc"` at an operator who may not
// write Go; named after the flag, because the message it produces names it.
type workersValue int

// newWorkersValue writes the default through p, as pflag's own *VarP helpers do.
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

// Type is the placeholder pflag prints in help text, as in `-w, --workers int`.
func (w *workersValue) Type() string { return "int" }

// String is what pflag prints for a set flag, and also what it records as
// DefValue, which is where the `(default N)` in the help line comes from.
func (w *workersValue) String() string { return strconv.Itoa(int(*w)) }

// parseWorkers parses a worker count. The range is deliberately not checked
// here — a value pflag's parser rejects is a usage error and cobra answers those
// with the flag list — so 0 and -1 parse, and validateRanges rejects them a
// moment later with usage silenced.
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
