package cli

import (
	"flag"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// acceptedValue is a spelling Set must take and what the flag prints afterwards.
type acceptedValue struct{ set, want string }

// TestFlagValues covers both flag.Value adapters as the flag package sees them:
// the default through the caller's pointer, the placeholder the help text is
// built from, every accepted spelling, and the message a rejected one produces.
func TestFlagValues(t *testing.T) {
	var (
		workers int
		timeout time.Duration
	)

	tests := []struct {
		name          string
		register      func(*flag.FlagSet)
		get           func() string
		wantType      string
		wantDef       string
		accepted      []acceptedValue
		badValue      string
		wantFragments []string
		// noStrconv is #50 itself: strconv's internals reaching the operator.
		noStrconv bool
	}{
		{
			name: "workers",
			register: func(fs *flag.FlagSet) {
				fs.Var(newWorkersValue(8, &workers), "workers", "number of parallel workers")
			},
			get:           func() string { return strconv.Itoa(workers) },
			wantType:      "int",
			wantDef:       "8",
			accepted:      []acceptedValue{{set: "4", want: "4"}},
			badValue:      "abc",
			wantFragments: []string{"workers", "abc", "want a whole number"},
			noStrconv:     true,
		},
		{
			name: "timeout",
			register: func(fs *flag.FlagSet) {
				fs.Var(newDurationValue(90*time.Second, &timeout), "timeout", "how long to run")
			},
			get:      func() string { return timeout.String() },
			wantType: "duration",
			wantDef:  "1m30s",
			// The bare-seconds spelling has no workersValue counterpart.
			accepted:      []acceptedValue{{set: "5m", want: "5m0s"}, {set: "300", want: "5m0s"}},
			badValue:      "5 minutes",
			wantFragments: []string{"timeout", "5 minutes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			tt.register(fs)

			f := fs.Lookup(tt.name)
			if f == nil {
				t.Fatalf("Lookup(%q) = nil, want the registered flag", tt.name)
			}

			// The flag package has no use for Type; the Flags block does, as the
			// placeholder in `-t, --timeout duration`.
			placeholder, ok := f.Value.(interface{ Type() string })
			if !ok {
				t.Fatalf("%s value has no Type method, which is what --help prints after the flag name", tt.name)
			}
			if got := placeholder.Type(); got != tt.wantType {
				t.Errorf("%s type = %q, want %q", tt.name, got, tt.wantType)
			}

			// Recorded by the flag package at registration, from String.
			if f.DefValue != tt.wantDef {
				t.Errorf("%s default = %q, want %q", tt.name, f.DefValue, tt.wantDef)
			}
			if got := tt.get(); got != tt.wantDef {
				t.Errorf("%s = %q, want the default written through the pointer", tt.name, got)
			}

			for _, a := range tt.accepted {
				if err := fs.Set(tt.name, a.set); err != nil {
					t.Fatalf("Set(%q) error = %v, want nil", a.set, err)
				}
				if got := tt.get(); got != a.want {
					t.Errorf("%s = %q after Set(%q), want %q", tt.name, got, a.set, a.want)
				}
				if got := f.Value.String(); got != a.want {
					t.Errorf("%s prints as %q after Set(%q), want %q", tt.name, got, a.set, a.want)
				}
			}

			// Through Parse rather than Set, because the message an operator
			// reads is the one the flag package builds around this one.
			err := fs.Parse([]string{"-" + tt.name, tt.badValue})
			if err == nil {
				t.Fatalf("Parse(-%s %q) error = nil, want a parse error", tt.name, tt.badValue)
			}

			for _, fragment := range tt.wantFragments {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("Parse(-%s %q) error = %q, want it to contain %q", tt.name, tt.badValue, err, fragment)
				}
			}

			if tt.noStrconv && strings.Contains(err.Error(), "strconv") {
				t.Errorf("Parse(-%s %q) error = %q, want no strconv internals in it (#50)", tt.name, tt.badValue, err)
			}
		})
	}
}

// TestBoolValueTakesNoArgument is what IsBoolFlag buys: without it `stressy -h`
// reads the next word as --help's value, and `stressy -h -w 4` loses the 4.
func TestBoolValueTakesNoArgument(t *testing.T) {
	var help bool

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(newBoolValue(&help), "help", "help for stressy")

	if err := fs.Parse([]string{"-help", "rest"}); err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}

	if !help {
		t.Error("--help = false after `-help`, want true")
	}

	if args := fs.Args(); len(args) != 1 || args[0] != "rest" {
		t.Errorf("Args() = %q, want the word after `-help` left as an argument", args)
	}
}
