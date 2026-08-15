//go:build unix

// An exit code is a claim about a process, and nothing inside a test binary can
// observe os.Exit — main() ends the process rather than returning to a caller
// that could inspect it. These cases therefore re-exec the test binary as the
// child half of each run and read the status the operating system reports, which
// is the same thing a Kubernetes Job, a `docker run` or a shell reads.
//
// Kept out of the Windows build: releases cross-compile there, and signalling a
// process is not supported.

package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	// childEnv marks the child half; childArgsEnv carries its command line.
	childEnv     = "GO_STRESSY_EXIT_TEST"
	childArgsEnv = "GO_STRESSY_EXIT_TEST_ARGS"

	// readmePath is named in the failures below: the shapes checked here are
	// the ones its sample sessions show.
	readmePath = "README.md"

	// stopHint and helpPointer bracket the line an indefinite run prints (#52).
	stopHint    = "Press Ctrl+C or send SIGTERM to stop."
	helpPointer = "Use --help for additional information"

	// drainLine is the tail of both shutdown lines: what the run waits for
	// between that line and the summary, which is the whole of what a SIGKILLed
	// pod ever gets to say about the wait it died in (#122). Spelled out here
	// rather than imported, this being package main.
	drainLine = "shutting down; waiting for every worker to finish the hash it is on..."

	// exitBudget bounds one child end to end. Generous on purpose: a `-t 2s`
	// run still has to wait out the hash its worker is inside, which measures
	// ~0.18s on an M-series core and ~1.9s on a loaded CI runner under -race —
	// the same reason internal/stressy's stopBudget is 30s.
	exitBudget = 60 * time.Second
)

var (
	// summaryLine is the shape of the line a finished run prints (#49).
	summaryLine = regexp.MustCompile(`^Computed (\d+) hash(?:es)? in \S+ \(\d+\.\d+ hashes/s, \d+ workers?\)$`)
	// progressLine is the same for the --report heartbeat (#70).
	progressLine = regexp.MustCompile(`^\S+ elapsed, (\d+) hash(?:es)?, \d+\.\d+ hashes/s$`)
)

// TestExitCodes covers #48: a signalled run exited 0, so an evicted pod read Complete.
func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args string
		// sig is sent once the child has printed its startup line. Zero sends none.
		sig         syscall.Signal
		wantCode    int
		wantLines   []string // prefixes that must appear, in this order
		wantNoLines []string
		wantHashes  bool
		// wantProgress requires the heartbeat; unset requires silence (#70).
		wantProgress bool
		wantStderr   string // empty where the run must not have reported an error
	}{
		// Two seconds so a hash certainly finished; no help pointer (#52).
		{
			name:        "a run that finishes its timeout",
			args:        "-w 1 -t 2s",
			wantLines:   []string{"Starting CPU stress test with 1 worker for 2s", "Timer expired, " + drainLine, "Computed "},
			wantNoLines: []string{helpPointer},
			wantHashes:  true,
		},
		// Three seconds against the 1s floor `--report` has had since #114, so
		// two ticks fall inside the run.
		{
			name:         "a run that reports its progress",
			args:         "-w 1 -t 3s --report 1s",
			wantLines:    []string{"Starting CPU stress test with 1 worker for 3s", "Timer expired, " + drainLine, "Computed "},
			wantHashes:   true,
			wantProgress: true,
		},
		// #114 and #115 end a run before it starts, and say which bound it missed.
		{name: "a report interval under the floor", args: "-w 1 -t 3s -r 100ms", wantCode: 1, wantStderr: "report must be 0 (off) or 1s or greater"},
		{name: "a report interval longer than the run", args: "-w 1 -t 3s -r 1m", wantCode: 1, wantStderr: "report 1m0s is longer than timeout 3s"},
		{
			name:      "an indefinite run says how to stop it",
			args:      "-w 1",
			sig:       syscall.SIGTERM,
			wantCode:  143,
			wantLines: []string{"Starting CPU stress test with 1 worker indefinitely", stopHint, "Received SIGTERM, " + drainLine, "Computed "},
		},
		{
			name:      "SIGINT",
			args:      "-w 1 -t 10m",
			sig:       syscall.SIGINT,
			wantCode:  130,
			wantLines: []string{"Starting CPU stress test with 1 worker for 10m0s", "Received SIGINT, " + drainLine, "Computed "},
		},
		// Unchanged by #48: `-w 0` fails the range check, `--bogus` the parser.
		{name: "a configuration the command rejects", args: "-w 0", wantCode: 1, wantStderr: "workers must be 1 or greater"},
		// #143: sync.WaitGroup counts in an int32, so this count wrapped its
		// counter negative and the run died on `panic: sync: negative WaitGroup
		// counter` — a stack trace, a startup line with no summary under it, and
		// exit 2, which is a code this table does not carry. The 1 here is the
		// row that says an out-of-range value is rejected and no work is done.
		{
			name: "a worker count no WaitGroup can hold", args: "-w 2147483648 -t 1ns", wantCode: 1,
			wantNoLines: []string{"Starting CPU stress test"},
			wantStderr:  "workers must be 2147483647 or fewer",
		},
		// The flag package names the flag single-dashed, whichever way it was spelled.
		{name: "a flag the command does not have", args: "--bogus", wantCode: 1, wantStderr: "flag provided but not defined: -bogus"},
		// #124: both answered above the operand check and exited 0 with the word
		// discarded, which is what made the 1 below true of neither command line.
		{
			name: "an argument after --help", args: "-h 4", wantCode: 1,
			wantNoLines: []string{"Stressy is a lightweight"},
			wantStderr:  `unexpected argument "4"`,
		},
		{
			name: "arguments after --version", args: "-v extra junk", wantCode: 1,
			wantNoLines: []string{"stressy version"},
			wantStderr:  `unexpected argument "extra"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := runChild(t, tt.args, tt.sig, 0)
			code, stdout, stderr := child.code, child.stdout, child.stderr

			if child.killedBy != 0 {
				t.Fatalf("`stressy %s` was killed by %v rather than exiting, so it reported no code of its own", tt.args, child.killedBy)
			}

			if code != tt.wantCode {
				t.Errorf("`stressy %s` exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", tt.args, code, tt.wantCode, strings.Join(stdout, "\n"), stderr)
			}

			if !containsInOrder(stdout, tt.wantLines) {
				t.Errorf("`stressy %s` printed:\n%s\nwant these lines, in this order: %q", tt.args, strings.Join(stdout, "\n"), tt.wantLines)
			}

			printed := strings.Join(stdout, "\n")
			for _, unwanted := range tt.wantNoLines {
				if strings.Contains(printed, unwanted) {
					t.Errorf("`stressy %s` printed:\n%s\nwant no %q in it", tt.args, printed, unwanted)
				}
			}

			if tt.wantStderr == "" {
				// Run printed the shutdown line; reporting it here makes two.
				if strings.Contains(stderr, "Error:") {
					t.Errorf("`stressy %s` printed %q on stderr, want the shutdown reported once (#48)", tt.args, stderr)
				}
			} else if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("`stressy %s` stderr = %q, want it to contain %q", tt.args, stderr, tt.wantStderr)
			}

			checkSummary(t, tt.args, stdout, tt.wantHashes)
			checkProgress(t, tt.args, stdout, tt.wantProgress)
		})
	}
}

// TestASecondSignalEndsTheDrain covers #122. A signalled run does not stop until
// every worker has finished the bcrypt hash it is inside, and past GOMAXPROCS
// those finish in series, so the wait grows with `-w`: about 0.2s at `-w 18` on
// 18 cores, about 20s at `-w 2000`. Through all of it the handler stayed
// installed and nothing read the channel again, so a second Ctrl-C landed in a
// one-deep buffer and the run could be stopped by nothing but SIGKILL — which is
// what a pod's 30-second grace period and `docker stop`'s 10 send, and neither
// leaves a summary or the 143 the exit-code table is built on.
//
// Four workers, because the window this has to land in is one hash wide on a
// machine with cores to spare — three orders of magnitude more than the parent
// needs to read a line and signal on it — and more workers only widen it.
func TestASecondSignalEndsTheDrain(t *testing.T) {
	const args = "-w 4"

	child := runChild(t, args, syscall.SIGTERM, syscall.SIGINT)

	printed := strings.Join(child.stdout, "\n")

	if child.killedBy != syscall.SIGINT {
		t.Fatalf("`stressy %s` exited %d rather than being killed by the SIGINT sent during its drain, want a shutdown a second signal can interrupt (#122)\nstdout:\n%s", args, child.code, printed)
	}

	// The first signal was still handled: what the second one interrupts is the
	// drain, not the shutdown, and the log has to say which signal arrived.
	if !containsInOrder(child.stdout, []string{"Starting CPU stress test with 4 workers indefinitely", "Received SIGTERM, " + drainLine}) {
		t.Errorf("`stressy %s` printed:\n%s\nwant the shutdown it was still able to report", args, printed)
	}

	// And the cost, which is the trade #122 accepted: a run killed mid-drain
	// never reaches its summary.
	for _, line := range child.stdout {
		if strings.HasPrefix(line, "Computed ") {
			t.Errorf("`stressy %s` printed %q, want a run killed inside its drain to have got no further than the shutdown line", args, line)
		}
	}
}

// TestExitCodesChild is the process TestExitCodes re-execs; otherwise it skips.
func TestExitCodesChild(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		t.Skip("the child half of TestExitCodes; runs only when re-execed")
	}

	// Rewritten rather than passed, so the child parses flags as a real run does.
	os.Args = append(os.Args[:1], strings.Fields(os.Getenv(childArgsEnv))...)

	main()
}

// childRun is what the operating system reported for one re-execed child, and
// the two streams it wrote. killedBy is the finding of its own for #122: a run
// the second signal ends is killed rather than exiting, so it reports no code.
type childRun struct {
	code     int
	killedBy syscall.Signal
	stdout   []string
	stderr   string
}

// runChild runs `stressy args` in a re-execed copy of this binary. onStartup is
// sent once the child has printed its startup line, and onShutdown once it has
// printed its shutdown line; either may be 0 to send none.
//
// The second is delivered against a line rather than a delay because that line
// is the sequencing: the run stops handling signals before printing it, so a
// signal sent after reading it is one the drain cannot swallow.
func runChild(t *testing.T, args string, onStartup, onShutdown syscall.Signal) childRun {
	t.Helper()

	child := exec.Command(os.Args[0], "-test.run=^TestExitCodesChild$")

	child.Env = append(os.Environ(), childEnv+"=1", childArgsEnv+"="+args)

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

	// Read to EOF in the background: past each line before signalling on it.
	var (
		lines    []string
		scanErr  error
		running  = make(chan struct{})
		draining = make(chan struct{})
		drained  = make(chan struct{})
	)
	go func() {
		defer close(drained)

		scanner := bufio.NewScanner(out)
		started, shuttingDown := false, false

		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)

			if !started && strings.HasPrefix(line, "Starting CPU stress test") {
				started = true
				close(running)
			}

			if !shuttingDown && strings.Contains(line, drainLine) {
				shuttingDown = true
				close(draining)
			}
		}

		scanErr = scanner.Err()
	}()

	// One signal per line is enough: the handler precedes the startup line, and
	// the killedBy field below tells a kill from an exit.
	signalOn(t, child, onStartup, running, drained, "startup")
	signalOn(t, child, onShutdown, draining, drained, "shutdown")

	select {
	case <-drained:
	case <-time.After(exitBudget):
		t.Fatalf("child did not exit within %s of the signals it was sent (%v, then %v)", exitBudget, onStartup, onShutdown)
	}

	if scanErr != nil {
		t.Fatalf("reading the child's stdout: %v", scanErr)
	}

	// Read past Wait, not before it: Wait is what waits out the goroutine
	// os/exec copies stderr with, and reading the builder under it is a race.
	waitErr := child.Wait()

	result := childRun{stdout: lines, stderr: errOut.String()}

	if waitErr == nil {
		return result
	}

	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("Wait() error = %v, want the child's exit status", waitErr)
	}

	// A child killed by a signal reports no code of its own; which signal is
	// the finding, so it comes back rather than failing here (#122).
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		result.killedBy = status.Signal()

		return result
	}

	result.code = exitErr.ExitCode()

	return result
}

// signalOn sends sig once the child has printed the line ready waits on, and
// does nothing where sig is 0. A child that exits first is not an error here:
// what it exited with is the finding, and the caller reads it.
func signalOn(t *testing.T, child *exec.Cmd, sig syscall.Signal, ready, exited <-chan struct{}, line string) {
	t.Helper()

	if sig == 0 {
		return
	}

	select {
	case <-ready:
	case <-exited:
		return
	case <-time.After(exitBudget):
		t.Fatalf("child printed no %s line within %s", line, exitBudget)
	}

	if err := child.Process.Signal(sig); err != nil {
		t.Fatalf("Signal(%v) error = %v", sig, err)
	}
}

// checkSummary holds the line to the shape the README documents (#49).
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

// checkProgress holds the heartbeat to its shape and to #70's gap.
func checkProgress(t *testing.T, args string, stdout []string, want bool) {
	t.Helper()

	var (
		progress int
		last     = -1
		shutdown = -1
	)

	for i, line := range stdout {
		switch {
		case progressLine.MatchString(line):
			progress++
			last = i
		case strings.Contains(line, drainLine):
			shutdown = i
		}
	}

	if !want {
		if progress > 0 {
			t.Errorf("`stressy %s` printed:\n%s\nwant no progress line from a run that asked for none (#70)", args, strings.Join(stdout, "\n"))
		}

		return
	}

	if progress == 0 {
		t.Errorf("`stressy %s` printed:\n%s\nwant a progress line in the shape %s documents (#70)", args, strings.Join(stdout, "\n"), readmePath)

		return
	}

	if shutdown < 0 || last > shutdown {
		t.Errorf("`stressy %s` printed:\n%s\nwant every progress line before the shutdown line (#70)", args, strings.Join(stdout, "\n"))
	}
}

// containsInOrder reports whether every prefix in want matches a line, in order.
func containsInOrder(lines, want []string) bool {
	next := 0
	for _, line := range lines {
		if next < len(want) && strings.HasPrefix(line, want[next]) {
			next++
		}
	}

	return next == len(want)
}
