//go:build unix

// A coverage figure in a document is a measurement, and a measurement is the
// kind of claim that goes stale while nobody touches it: `internal/stressy`
// kept growing tests after the entry quoting its coverage was written, and the
// number in the entry did not move with them (#62). This re-measures it, by
// running the command the entry's reader would run.
//
// Kept out of the Windows build for the reason main_exit_test.go and
// internal/stressy/stressy_signal_test.go are kept out of it: signalling a
// process is not supported there, so stressy_signal_test.go is not in that
// build and the package measures lower. The figure the changelog publishes is
// the one a full test run produces, so it is checked where a full test run
// happens.

package main

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// coveragePercent pulls the figure out of `go test`'s own summary line rather
// than recomputing it from the profile: what the changelog quotes is the number
// a reader gets by running the command, down to how Go rounds it.
var coveragePercent = regexp.MustCompile(`coverage: (\d+\.\d+)% of statements`)

// profileBlock matches a line of a coverprofile: the file, the range it covers
// as `startLine.col,endLine.col`, how many statements are in it, and how many
// times they ran.
var profileBlock = regexp.MustCompile(`^(\S+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$`)

// coverBlock is one uncovered range, with the file made relative to the
// repository root — a coverprofile names it by import path.
type coverBlock struct {
	file       string
	start, end int
}

// TestUnreleasedCoverageClaim covers #62. The `[Unreleased]` entry said 24.2%
// to 97.3% and the package measured higher, because the tests that landed after
// the entry was written raised it and nobody re-ran the command. That number is
// the one shipping in the next release's changelog, and it is not the one a
// reader who re-runs the command gets.
//
// Both halves of the claim are checked, because the sentence makes two: the
// percentage, and that what is left uncovered is a single unreachable `panic`.
// The second is the interesting one — it is what says the missing 1.3% is a
// branch that cannot be reached rather than a path nobody got to, and it would
// quietly stop being true the first time a real gap opened.
func TestUnreleasedCoverageClaim(t *testing.T) {
	var claims []string

	for _, line := range unreleased(t) {
		if strings.Contains(line, "`"+stressyPkg+"`") && strings.Contains(line, "coverage") {
			claims = append(claims, line)
		}
	}

	// Nothing to check once the entry moves under a version heading, which is
	// the point of reading only `[Unreleased]`. See unreleased.
	if claims == nil {
		return
	}

	measured, uncovered := measureCoverage(t)

	for _, claim := range claims {
		if !strings.Contains(claim, measured+"%") {
			t.Errorf("%s: an [Unreleased] entry quotes coverage for %s without naming the %s%% a fresh `go test -cover ./%s` prints; re-measure it before the entry ships (#62)\n\t%s", changelogPath, stressyPkg, measured, stressyPkg, claim)
		}

		if !strings.Contains(claim, "unreachable") || !strings.Contains(claim, "panic") {
			continue
		}

		checkRemainderIsOnePanic(t, uncovered, claim)
	}
}

// checkRemainderIsOnePanic holds the second half of the entry's sentence: that
// the statements the tests do not reach are one unreachable `panic`.
func checkRemainderIsOnePanic(t *testing.T, uncovered []coverBlock, claim string) {
	t.Helper()

	if len(uncovered) != 1 {
		t.Errorf("%s: an [Unreleased] entry says the uncovered remainder of %s is one unreachable `panic`, where the coverprofile has %d uncovered blocks: %v (#62)\n\t%s", changelogPath, stressyPkg, len(uncovered), uncovered, claim)

		return
	}

	block := uncovered[0]

	src := lines(t, block.file)
	if block.end > len(src) {
		t.Fatalf("%s: the coverprofile names lines %d-%d, which the file does not have", block.file, block.start, block.end)
	}

	for _, line := range src[block.start-1 : block.end] {
		if strings.Contains(line, "panic(") {
			return
		}
	}

	t.Errorf("%s: an [Unreleased] entry says the uncovered remainder of %s is one unreachable `panic`, where the one uncovered block is %s:%d-%d and panics nowhere (#62)\n\t%s", changelogPath, stressyPkg, block.file, block.start, block.end, claim)
}

// measureCoverage runs the package's tests and returns the percentage `go test`
// reports and the blocks nothing reached. One run answers both, and it is the
// run the changelog entry describes rather than a reconstruction of it.
func measureCoverage(t *testing.T) (string, []coverBlock) {
	t.Helper()

	profile := t.TempDir() + "/cover.out"

	out, err := exec.Command("go", "test", "-coverprofile="+profile, "./"+stressyPkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go test -coverprofile ./%s = %v, want a passing run:\n%s", stressyPkg, err, out)
	}

	percent := coveragePercent.FindSubmatch(out)
	if percent == nil {
		t.Fatalf("go test -coverprofile ./%s printed no coverage figure, so the changelog's cannot be checked against it:\n%s", stressyPkg, out)
	}

	var uncovered []coverBlock

	prefix := modulePath(t) + "/"

	for i, line := range lines(t, profile) {
		block := profileBlock.FindStringSubmatch(line)

		// The first line is the `mode:` header and the last is empty; anything
		// else this does not recognise is a format it was not written for.
		if block == nil {
			if i > 0 && strings.TrimSpace(line) != "" {
				t.Fatalf("%s:%d: %q is not a coverprofile block, which this test only reads in the `file.go:1.2,3.4 5 6` form", profile, i+1, line)
			}

			continue
		}

		if block[4] != "0" {
			continue
		}

		uncovered = append(uncovered, coverBlock{
			// A coverprofile names files by import path, and these are read
			// off disk relative to the repository root, which is where `go
			// test` runs.
			file:  strings.TrimPrefix(block[1], prefix),
			start: atoi(t, block[2]),
			end:   atoi(t, block[3]),
		})
	}

	return string(percent[1]), uncovered
}

// modulePath is the import path a coverprofile names files by, read out of
// go.mod rather than written down here: the profile's paths are
// module-qualified and the files behind them are read back off disk, so the two
// spellings have to agree.
func modulePath(t *testing.T) string {
	t.Helper()

	for _, line := range lines(t, "go.mod") {
		if path, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(path)
		}
	}

	t.Fatal("go.mod has no `module` line, so a coverprofile's paths cannot be made relative to the repository")

	return ""
}

func atoi(t *testing.T, s string) int {
	t.Helper()

	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("Atoi(%q) = %v, want a line number out of the coverprofile", s, err)
	}

	return n
}
