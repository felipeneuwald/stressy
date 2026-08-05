//go:build unix

// An exit code is a claim about a process, and nothing inside a test binary can
// observe os.Exit — main() ends the process rather than returning to a caller
// that could inspect it. These cases therefore re-exec the test binary as the
// child half of each run and read the status the operating system reports, which
// is the same thing a Kubernetes Job, a `docker run` or a shell reads.
//
// Kept out of the Windows build for the reason
// internal/stressy/stressy_signal_test.go is: releases cross-compile there, and
// signalling a process is not supported. What that costs is coverage of exit 0
// and exit 1 on Windows, which are the two codes that did not change.

package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	// childEnv marks a re-execed test binary as the child half of
	// TestExitCodes, and childArgsEnv carries the command line it runs stressy
	// with: os.Args in the child is the test binary's own, so the invocation
	// has to travel out of band. Neither is spelled STRESSY_something. bindEnv
	// reads only STRESSY_ plus a flag name and would ignore them either way,
	// but a variable that looks like a stressy setting and is not one is a
	// thing to trip over rather than to explain.
	childEnv     = "GO_STRESSY_EXIT_TEST"
	childArgsEnv = "GO_STRESSY_EXIT_TEST_ARGS"

	// exitBudget bounds one child end to end. Generous on purpose: a `-t 2s`
	// run still has to wait out the hash its worker is inside, which measures
	// ~0.18s on an M-series core and ~1.9s on a loaded CI runner under -race —
	// the same reason internal/stressy's stopBudget is 30s.
	exitBudget = 60 * time.Second
)

// TestExitCodes covers #48 at the level the issue is about. Every one of these
// codes was reachable before it except the two that matter: a run cut short by
// SIGINT or SIGTERM exited 0, exactly like a run that served its whole `-t`, so
// a Job whose pod was evicted five seconds into a sixty-second run was recorded
// Complete and the `backoffLimit: 0` the README's manifest sets never saw the
// failure it exists to bound.
//
// The output assertions ride along because the same runs are the only place the
// end-of-run summary (#49) can be seen printed by a real process, on both
// shutdown paths, in the order it has to appear in: the summary is what closes
// the "shutting down..." ellipsis, so it comes after it.
func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		// args is the stressy command line, as it would be typed.
		args string
		// sig is sent once the child has printed its startup line, which is
		// what proves the handler is installed. Zero means send nothing.
		sig      syscall.Signal
		wantCode int
		// wantLines are prefixes that must appear on stdout, in this order.
		wantLines []string
		// wantNoLines are fragments that must appear nowhere on stdout. A line
		// that stopped printing is invisible to wantLines, which is how `Use
		// --help for additional information` came to print on every run for the
		// life of the project with no test either asking for it or objecting
		// (#52).
		wantNoLines []string
		// wantHashes requires the summary to report work actually done, which
		// only a run long enough to finish a hash can promise.
		wantHashes bool
		// wantStderr is a fragment cobra must have printed; empty means the run
		// must not have reported an error at all.
		wantStderr string
	}{
		{
			// Two seconds rather than something shorter so the worker is
			// certain to have finished a hash: the count is the difference
			// between a summary that reports the run and one that reports the
			// scheduler.
			name:     "a run that finishes its timeout",
			args:     "-w 1 -t 2s",
			wantCode: 0,
			wantLines: []string{
				"Starting CPU stress test with 1 worker for 2s",
				"Timer expired, shutting down...",
				"Computed ",
			},
			// A bounded run is a run whose operator has already read the flags.
			// This is also where the line was most often read and least use:
			// the Job log the README's Kubernetes workflow produces (#52).
			wantNoLines: []string{helpPointer},
			wantHashes:  true,
		},
		{
			// The other half of #52, and the only place it can be seen printed
			// by a real process: a run with no `-t` says how to stop it, where
			// it used to announce "indefinitely" and leave the reader to guess.
			// Ended by the signal it names, which is what makes the line's
			// claim testable rather than decorative.
			name:     "an indefinite run says how to stop it",
			args:     "-w 1",
			sig:      syscall.SIGTERM,
			wantCode: 143,
			wantLines: []string{
				"Starting CPU stress test with 1 worker indefinitely",
				stopHint,
				"Received signal, shutting down...",
				"Computed ",
			},
		},
		{
			// 128 + 2. What Ctrl-C at a terminal has always produced from any
			// process that does not handle it.
			name:     "SIGINT",
			args:     "-w 1 -t 10m",
			sig:      syscall.SIGINT,
			wantCode: 130,
			wantLines: []string{
				"Starting CPU stress test with 1 worker for 10m0s",
				"Received signal, shutting down...",
				"Computed ",
			},
		},
		{
			// 128 + 15. What a `docker stop`, a `kubectl delete pod`, a node
			// drain and an eviction all send.
			name:     "SIGTERM",
			args:     "-w 1 -t 10m",
			sig:      syscall.SIGTERM,
			wantCode: 143,
			wantLines: []string{
				"Starting CPU stress test with 1 worker for 10m0s",
				"Received signal, shutting down...",
				"Computed ",
			},
		},
		{
			// Unchanged by #48, and worth pinning next to the codes that did
			// change: a configuration stressy will not run is still a failure,
			// not a signal, and it is still 1.
			name:       "a configuration the command rejects",
			args:       "-w 0",
			wantCode:   1,
			wantStderr: "workers must be 1 or greater",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runChild(t, tt.args, tt.sig)

			if code != tt.wantCode {
				t.Errorf("`stressy %s` exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", tt.args, code, tt.wantCode, strings.Join(stdout, "\n"), stderr)
			}

			if !containsInOrder(stdout, tt.wantLines) {
				t.Errorf("`stressy %s` printed:\n%s\nwant these lines, in this order: %q", tt.args, strings.Join(stdout, "\n"), tt.wantLines)
			}

			printed := strings.Join(stdout, "\n")
			for _, unwanted := range tt.wantNoLines {
				if strings.Contains(printed, unwanted) {
					t.Errorf("`stressy %s` printed:\n%s\nwant no %q in it (#52)", tt.args, printed, unwanted)
				}
			}

			if tt.wantStderr == "" {
				// The other half of not double-reporting the shutdown: Run has
				// already said "Received signal, shutting down...", so cobra
				// printing `Error: run interrupted by interrupt` under it would
				// report one shutdown twice, the second time as a failure.
				if strings.Contains(stderr, "Error:") {
					t.Errorf("`stressy %s` printed %q on stderr, want the shutdown reported once (#48)", tt.args, stderr)
				}
			} else if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("`stressy %s` stderr = %q, want it to contain %q", tt.args, stderr, tt.wantStderr)
			}

			checkSummary(t, tt.args, stdout, tt.wantHashes)
		})
	}
}

// TestExitCodesChild is not a test of its own. It is the process TestExitCodes
// re-execs: with the marker set it runs the real main(), so the child exits the
// way stressy exits and the status the parent reads is the one an operator gets.
// Every ordinary `go test` run finds no marker and does nothing.
func TestExitCodesChild(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		t.Skip("the child half of TestExitCodes; runs only when re-execed")
	}

	// Rewritten rather than handed to the command directly: cobra reads os.Args
	// itself when nothing overrides it, and os.Args here is the test binary's
	// own command line. Going through it keeps the child on the same path a
	// real invocation takes, flag parsing included, and the testing package has
	// finished with os.Args long before any test runs.
	os.Args = append(os.Args[:1], strings.Fields(os.Getenv(childArgsEnv))...)

	main()
}

// runChild runs `stressy args` in a re-execed copy of this test binary,
// optionally signalling it once it is running, and returns the exit status it
// reported along with everything it printed.
func runChild(t *testing.T, args string, sig syscall.Signal) (code int, stdout []string, stderr string) {
	t.Helper()

	child := exec.Command(os.Args[0], "-test.run=^TestExitCodesChild$")

	// The ambient environment configures a run too, so a developer with
	// STRESSY_TIMEOUT exported would otherwise be testing something other than
	// the command line under test. Same care as docs_test.go takes.
	for _, assignment := range os.Environ() {
		if !strings.HasPrefix(assignment, "STRESSY_") {
			child.Env = append(child.Env, assignment)
		}
	}
	child.Env = append(child.Env, childEnv+"=1", childArgsEnv+"="+args)

	var errOut strings.Builder
	child.Stderr = &errOut

	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe() error = %v", err)
	}

	if err := child.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })

	// Read to EOF in the background: the parent has to be past the startup line
	// before it signals, and past EOF before it waits.
	var (
		lines   []string
		scanErr error
		running = make(chan struct{})
		drained = make(chan struct{})
	)
	go func() {
		defer close(drained)

		scanner := bufio.NewScanner(out)
		started := false

		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)

			if !started && strings.HasPrefix(line, "Starting CPU stress test") {
				started = true

				close(running)
			}
		}

		scanErr = scanner.Err()
	}()

	if sig != 0 {
		select {
		case <-running:
		case <-drained:
			// The child exited before it ever started a run; the exit code
			// below is the finding, so let it be reported rather than timing
			// out here.
		case <-time.After(exitBudget):
			t.Fatalf("child printed no startup line within %s", exitBudget)
		}

		// One signal is enough because the handler is registered before that
		// line is printed. Sent any earlier, the default disposition would kill
		// the child outright and the status would be "killed by SIGTERM"
		// rather than the code stressy chose — which is what the check on
		// WaitStatus.Signaled below tells apart.
		if err := child.Process.Signal(sig); err != nil {
			t.Fatalf("Signal(%v) error = %v", sig, err)
		}
	}

	select {
	case <-drained:
	case <-time.After(exitBudget):
		t.Fatalf("child did not exit within %s of %v", exitBudget, sig)
	}

	if scanErr != nil {
		t.Fatalf("reading the child's stdout: %v", scanErr)
	}

	if err := child.Wait(); err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if !ok {
			t.Fatalf("Wait() error = %v, want the child's exit status", err)
		}

		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			t.Fatalf("child was killed by %v rather than exiting, so it reported no code of its own", status.Signal())
		}

		code = exitErr.ExitCode()
	}

	return code, lines, errOut.String()
}

// checkSummary holds the line a real run prints to the shape docs_test.go
// checks the README's sample session against, so the documented output and the
// printed one cannot drift apart (#49).
func checkSummary(t *testing.T, args string, stdout []string, wantHashes bool) {
	t.Helper()

	for _, line := range stdout {
		if !strings.HasPrefix(line, "Computed ") {
			continue
		}

		match := summaryLine.FindStringSubmatch(line)
		if match == nil {
			t.Errorf("`stressy %s` printed %q, which is not the shape %s documents", args, line, readmePath)

			return
		}

		if !wantHashes {
			return
		}

		hashes, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("hash count %q is not a number: %v", match[1], err)
		}

		if hashes == 0 {
			t.Errorf("`stressy %s` reported %q, want a run given two seconds to have computed at least one hash", args, line)
		}

		return
	}
}

// containsInOrder reports whether every prefix in want matches a line, in the
// order given. Prefixes rather than whole lines because the summary carries
// measured numbers, and order because the summary is the confirmation the
// "shutting down..." ellipsis promises and has to follow it.
func containsInOrder(lines, want []string) bool {
	next := 0
	for _, line := range lines {
		if next < len(want) && strings.HasPrefix(line, want[next]) {
			next++
		}
	}

	return next == len(want)
}
