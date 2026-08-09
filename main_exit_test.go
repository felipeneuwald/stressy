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

	// exitBudget bounds one child end to end. Generous on purpose: a `-t 2s`
	// run still has to wait out the hash its worker is inside, which measures
	// ~0.18s on an M-series core and ~1.9s on a loaded CI runner under -race —
	// the same reason internal/stressy's stopBudget is 30s.
	exitBudget = 60 * time.Second
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
			wantLines:   []string{"Starting CPU stress test with 1 worker for 2s", "Timer expired, shutting down...", "Computed "},
			wantNoLines: []string{helpPointer},
			wantHashes:  true,
		},
		{
			name:         "a run that reports its progress",
			args:         "-w 1 -t 2s --report 250ms",
			wantLines:    []string{"Starting CPU stress test with 1 worker for 2s", "Timer expired, shutting down...", "Computed "},
			wantHashes:   true,
			wantProgress: true,
		},
		{
			name:      "an indefinite run says how to stop it",
			args:      "-w 1",
			sig:       syscall.SIGTERM,
			wantCode:  143,
			wantLines: []string{"Starting CPU stress test with 1 worker indefinitely", stopHint, "Received signal, shutting down...", "Computed "},
		},
		{
			name:      "SIGINT",
			args:      "-w 1 -t 10m",
			sig:       syscall.SIGINT,
			wantCode:  130,
			wantLines: []string{"Starting CPU stress test with 1 worker for 10m0s", "Received signal, shutting down...", "Computed "},
		},
		// Unchanged by #48: `-w 0` fails the range check, `--bogus` the parser.
		{name: "a configuration the command rejects", args: "-w 0", wantCode: 1, wantStderr: "workers must be 1 or greater"},
		// The flag package names the flag single-dashed, whichever way it was spelled.
		{name: "a flag the command does not have", args: "--bogus", wantCode: 1, wantStderr: "flag provided but not defined: -bogus"},
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

// TestExitCodesChild is the process TestExitCodes re-execs; otherwise it skips.
func TestExitCodesChild(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		t.Skip("the child half of TestExitCodes; runs only when re-execed")
	}

	// Rewritten rather than passed, so the child parses flags as a real run does.
	os.Args = append(os.Args[:1], strings.Fields(os.Getenv(childArgsEnv))...)

	main()
}

// runChild runs `stressy args` in a re-execed copy of this binary, optionally signalling it.
func runChild(t *testing.T, args string, sig syscall.Signal) (code int, stdout []string, stderr string) {
	t.Helper()

	child := exec.Command(os.Args[0], "-test.run=^TestExitCodesChild$")

	// A developer with STRESSY_TIMEOUT exported would be testing something else.
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

	// Read to EOF in the background: past the startup line before signalling.
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
			// The child exited before starting; the exit code is the finding.
		case <-time.After(exitBudget):
			t.Fatalf("child printed no startup line within %s", exitBudget)
		}

		// One signal is enough: the handler precedes that line, and the
		// WaitStatus.Signaled check below tells a kill from an exit.
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

// checkSummary holds the line to the shape docs_test.go documents (#49).
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
		case strings.HasSuffix(line, ", shutting down..."):
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
