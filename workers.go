package main

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// workersValue adapts the worker count to the pflag.Value interface.
//
// pflag already ships IntVarP, and --workers used it — so an unparseable value
// came back as whatever strconv returned: `invalid argument "abc" for "-w,
// --workers" flag: strconv.ParseInt: parsing "abc": invalid syntax`. That names
// a Go standard library function to an operator who may not write Go, and says
// nothing about what a valid value would have looked like. The flag registered
// five lines below it in newCmd has said so in words since #26, and for exactly
// this reason; this is the same treatment (#50).
//
// Named after the flag rather than after its type, unlike durationValue,
// because the message names the setting: "invalid workers" is a thing the
// operator typed and can go and correct, where "invalid int" would say only
// that this is written in Go.
type workersValue int

// newWorkersValue writes the default through the caller's pointer, the same way
// pflag's own *VarP helpers do, and returns a pflag.Value aliasing it.
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

// Type is the value placeholder pflag prints in help text, as in
// `-w, --workers int`. It is what IntVarP printed: the help line is not what
// #50 is about, and an operator reading `--workers int` learns more from it
// than from `--workers workers`.
func (w *workersValue) Type() string { return "int" }

// String is both what pflag prints for a set flag and what it records as
// DefValue when the flag is registered, which is where the `(default 8)` in the
// help line comes from.
func (w *workersValue) String() string { return strconv.Itoa(int(*w)) }

// parseWorkers parses a worker count.
//
// The range is deliberately not checked here. A value pflag's parser rejects is
// a usage error, and cobra answers those with the flag list — `stressy -w 0`
// replying to a one-line configuration mistake with a page of flags is the
// whole of what #17a removed. So 0 and -1 parse, and validateRanges rejects
// them a moment later with usage silenced. The message still says what a valid
// value looks like, because that is what the operator needs on either path.
func parseWorkers(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err == nil {
		return n, nil
	}

	// strconv tells "not a number at all" apart from "a number no int can
	// hold", and so should this: telling someone who typed twenty digits that a
	// whole number is wanted reads as though they had not.
	if errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("invalid workers %q: out of range, want a whole number from 1 to %d", s, math.MaxInt)
	}

	return 0, fmt.Errorf("invalid workers %q: want a whole number, 1 or greater", s)
}
