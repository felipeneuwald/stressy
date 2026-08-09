package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/felipeneuwald/stressy/internal/cli"
	"github.com/felipeneuwald/stressy/internal/stressy"
)

// Every documented stressy invocation, run through the real command.

const (
	readmePath = "README.md"
	// CONTRIBUTING.md publishes an invocation of its own, `./stressy -w 2 -t 5s`.
	contributingPath  = "CONTRIBUTING.md"
	releaseConfigPath = ".goreleaser.yaml"
	ciWorkflowPath    = ".github/workflows/ci.yml"
	// A reference starting with imageRepo is stressy's own, not somebody else's.
	imageRepo = "ghcr.io/felipeneuwald/stressy"
	// stopHint and helpPointer bracket the line an indefinite run prints (#52).
	stopHint    = "Press Ctrl+C or send SIGTERM to stop."
	helpPointer = "Use --help for additional information"
	// A tag wrapped in these ships on a full release and on nothing else (#23).
	prereleaseGuard = "{{ if not .Prerelease }}"
	guardEnd        = "{{ end }}"
)

var (
	// imageRef admits no `{`, so the config's templated tags do not match it.
	imageRef = regexp.MustCompile(regexp.QuoteMeta(imageRepo) + `:[\w.\-]+`)
	// summaryLine is the shape of the line a finished run prints (#49).
	summaryLine = regexp.MustCompile(`^Computed (\d+) hash(?:es)? in \S+ \(\d+\.\d+ hashes/s, \d+ workers?\)$`)
	// progressLine is the same for the --report heartbeat (#70).
	progressLine = regexp.MustCompile(`^\S+ elapsed, (\d+) hash(?:es)?, \d+\.\d+ hashes/s$`)
	// exitCodeRow matches a row of the README's exit-code table.
	exitCodeRow = regexp.MustCompile("(?m)^\\|\\s*`(\\d+)`\\s*\\|\\s*(.+?)\\s*\\|\\s*$")
	// readmeSignalNames maps a signal to the name the README has to call it by.
	readmeSignalNames = map[os.Signal]string{
		syscall.SIGINT:  "SIGINT",
		syscall.SIGTERM: "SIGTERM",
	}
)

// invocation is a stressy run lifted out of the documentation.
type invocation struct {
	source        string
	env           []string
	args          []string
	containerised bool
}

// TestDocumentedInvocationsAreValid runs every documented command line.
func TestDocumentedInvocationsAreValid(t *testing.T) {
	for _, inv := range documentedInvocations(t) {
		t.Run(inv.source, func(t *testing.T) {
			inv.run(t)
		})
	}
}

// TestContainerExamplesAreBounded covers #27b: a container that never stops
// leaves a Kubernetes Job at 0/1 forever, never finishing and never cleaned up.
func TestContainerExamplesAreBounded(t *testing.T) {
	var checked int
	for _, inv := range documentedInvocations(t) {
		if !inv.containerised {
			continue
		}
		checked++
		t.Run(inv.source, func(t *testing.T) {
			if cfg := inv.run(t); cfg.Timeout == 0 {
				t.Errorf("%s: `%s` runs until interrupted, which in a container leaves nothing to interrupt it; give the example a -t or a STRESSY_TIMEOUT (#27b)", inv.source, strings.Join(inv.args, " "))
			}
		})
	}

	if checked == 0 {
		t.Error("no containerised examples found, so this test is checking nothing; either the README lost its Docker and Kubernetes sections or the parsing above stopped recognising them")
	}
}

// TestDocumentedImagesAreReleased checks the documented images are published.
func TestDocumentedImagesAreReleased(t *testing.T) {
	published := publishedImages(t)
	for _, ref := range documentedImages(t) {
		if !slices.Contains(published, ref) {
			t.Errorf("documentation names %s, which %s does not publish; it publishes %v", ref, releaseConfigPath, published)
		}
	}
}

// TestDocumentedExitCodes checks the README's exit-code table both ways (#48).
func TestDocumentedExitCodes(t *testing.T) {
	doc := strings.Join(lines(t, readmePath), "\n")
	rows := exitCodeRow.FindAllStringSubmatch(doc, -1)
	if rows == nil {
		t.Fatalf("%s documents no exit codes, so a supervisor reading one has nothing to look it up in (#48)", readmePath)
	}
	documented := make(map[int]string, len(rows))
	for _, row := range rows {
		code, err := strconv.Atoi(row[1])
		if err != nil {
			t.Fatalf("%s: exit code %q is not a number: %v", readmePath, row[1], err)
		}
		documented[code] = row[2]
	}

	// The two codes that predate #48: a completed run, and a rejected config.
	for _, code := range []int{0, 1} {
		if _, ok := documented[code]; !ok {
			t.Errorf("%s documents no exit code %d", readmePath, code)
		}
	}

	signalled := make(map[int]bool)
	for _, sig := range stressy.ShutdownSignals() {
		name, ok := readmeSignalNames[sig]
		if !ok {
			t.Errorf("stressy stops on %v, which this test knows no README name for; add it to readmeSignalNames", sig)

			continue
		}
		code := (&stressy.SignalError{Signal: sig}).ExitCode()
		signalled[code] = true
		meaning, ok := documented[code]
		if !ok {
			t.Errorf("a run ended by %s exits %d, which %s documents nowhere (#48)", name, code, readmePath)

			continue
		}
		if !strings.Contains(meaning, name) {
			t.Errorf("%s documents exit code %d as %q, which does not name %s", readmePath, code, meaning, name)
		}
	}

	for code := range documented {
		if code == 0 || code == 1 || signalled[code] {
			continue
		}
		t.Errorf("%s documents exit code %d, which no run produces", readmePath, code)
	}
}

// TestPrecedenceIsDocumentedWhereUsersRead is #47's live guard on `--help`.
//
// It walks cli.Settings() and never the flag set: every setting is registered
// there twice, once under each spelling, so a loop over the set would report
// each shorthand as a setting of its own that nothing documents.
func TestPrecedenceIsDocumentedWhereUsersRead(t *testing.T) {
	long := collapse(cli.Description)

	for _, s := range cli.Settings() {
		if s.EnvVar == "" {
			// Offering a variable the binding refuses to read reads as an
			// invitation to set it, which is worse than documenting nothing.
			if variable := wouldBeEnvVar(s); strings.Contains(long, variable) {
				t.Errorf("`stressy --help` offers %s, which bindEnv skips because --%s is not a setting a run is configured from (#47)", variable, s.Long)
			}

			continue
		}

		if !strings.Contains(long, "--"+s.Long) {
			t.Errorf("`stressy --help` says which flags read the environment without naming --%s, which bindEnv fills from %s (#64)", s.Long, s.EnvVar)
		}
	}
}

// wouldBeEnvVar is the variable a setting would be filled from if it were
// fillable at all. cli.Settings leaves EnvVar empty for --help and --version
// precisely because nothing reads them, and the name an operator would guess is
// still the one the documentation has to stay clear of.
func wouldBeEnvVar(s cli.Setting) string {
	if s.EnvVar != "" {
		return s.EnvVar
	}

	return "STRESSY_" + strings.ToUpper(s.Long)
}

// TestShellLineStripsTrailingComments covers the truncation at `#`. Every fence
// in the tree writes its comments on their own line, so no documented line
// reaches the case and removing the truncation would leave the rest of this
// file green — while `stressy -w 4  # four workers`, written by whoever next
// edits a fence, would run with `#` and `four` in its command line.
func TestShellLineStripsTrailingComments(t *testing.T) {
	tests := []struct {
		name string
		line string
		// want nil for a line that describes no invocation.
		want []string
	}{
		{name: "trailing comment", line: "stressy -w 4  # four workers", want: []string{"-w", "4"}},
		// The truncation has to run after the `$` strip, not before it.
		{name: "after a console prompt", line: "$ stressy -t 5m # bounded", want: []string{"-t", "5m"}},
		{name: "whole-line comment", line: "# a whole-line comment"},
		// slices.Index matches a standalone `#` only, which is what the
		// HasPrefix half of the skip is still there for.
		{name: "no space after the hash", line: "#four workers"},
		{name: "no comment", line: "stressy -w 4", want: []string{"-w", "4"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, ok := (&shell{}).line(t, "test", tt.line)
			if ok != (tt.want != nil) {
				t.Fatalf("line(%q) ok = %v, want %v", tt.line, ok, tt.want != nil)
			}
			if ok && !slices.Equal(inv.args, tt.want) {
				t.Errorf("line(%q) args = %q, want %q", tt.line, inv.args, tt.want)
			}
		})
	}
}

// run resolves the invocation through cli.Parse, which is the real parser, the
// real environment binding and the real range checks with the stress test left
// out: the run itself saturates the CPU and blocks until signalled, where these
// cases are about what a documented command line configures.
func (inv invocation) run(t *testing.T) stressy.Cfg {
	t.Helper()

	clearStressyEnv(t)

	for _, assignment := range inv.env {
		name, value, ok := strings.Cut(assignment, "=")
		if !ok {
			t.Fatalf("%s: %q is not a NAME=value assignment", inv.source, assignment)
		}
		t.Setenv(name, value)
	}

	var cfg stressy.Cfg
	if err := cli.Parse(&cfg, inv.args); err != nil {
		t.Fatalf("%s: documented invocation `%s` = %v, want it to work as written", inv.source, strings.Join(append(slices.Clone(inv.env), append([]string{"stressy"}, inv.args...)...), " "), err)
	}

	return cfg
}

// clearStressyEnv blanks every variable a run can be configured from, so a case
// reads the invocation in front of it rather than the one the shell running `go
// test` happens to export (#58). The names come off the flag table, so a fourth
// setting is covered the day it is registered; blank rather than unset, because
// bindEnv treats an empty value as unset deliberately and t.Setenv restores what
// was there afterwards. A documented value therefore has to be applied after
// this returns — the environment is read at parse time, so that costs nothing.
func clearStressyEnv(t *testing.T) {
	t.Helper()

	for _, s := range cli.Settings() {
		if s.EnvVar != "" {
			t.Setenv(s.EnvVar, "")
		}
	}
}

// documentedInvocations gathers every stressy run the project publishes.
// CONTRIBUTING.md is read too: a flag rename would leave it green (#59).
func documentedInvocations(t *testing.T) []invocation {
	t.Helper()
	var found []invocation
	for _, path := range []string{readmePath, contributingPath} {
		found = append(found, fileInvocations(t, path)...)
	}
	found = append(found, exampleInvocations(t)...)
	if len(found) == 0 {
		t.Fatal("no documented invocations found, so these tests are checking nothing")
	}

	return found
}

// fileInvocations reads a document's fenced blocks: shell, and YAML for the Job.
func fileInvocations(t *testing.T, path string) []invocation {
	t.Helper()
	var (
		found   []invocation
		inBlock bool
		lang    string
		sh      shell
		image   string
	)

	for i, line := range lines(t, path) {
		trimmed := strings.TrimSpace(line)
		source := fmt.Sprintf("%s:%d", path, i+1)
		if fence, ok := strings.CutPrefix(trimmed, "```"); ok {
			if inBlock {
				inBlock, lang, sh, image = false, "", shell{}, ""
			} else {
				inBlock, lang = true, fence
			}

			continue
		}
		if !inBlock {
			continue
		}
		switch lang {
		// console as well as bash: the sample session is an invocation too.
		case "bash", "console":
			if inv, ok := sh.line(t, source, trimmed); ok {
				found = append(found, inv)
			}
		case "yaml":
			// The one shape the Job manifest uses: `image:`, then `args:`.
			if ref, ok := strings.CutPrefix(trimmed, "image:"); ok {
				image = strings.TrimSpace(ref)

				continue
			}
			if list, ok := strings.CutPrefix(trimmed, "args:"); ok && strings.HasPrefix(image, imageRepo) {
				found = append(found, invocation{
					source:        source,
					args:          flowList(t, source, list),
					containerised: true,
				})
			}
		}
	}

	return found
}

// exampleInvocations reads the Examples block `stressy --help` prints (#27c).
func exampleInvocations(t *testing.T) []invocation {
	t.Helper()
	if strings.TrimSpace(cli.Examples) == "" {
		t.Fatal("the command has no Examples block, so `stressy --help` shows no examples (#27c)")
	}

	var (
		found []invocation
		sh    shell
	)
	for i, line := range strings.Split(cli.Examples, "\n") {
		if inv, ok := sh.line(t, fmt.Sprintf("--help:%d", i+1), strings.TrimSpace(line)); ok {
			found = append(found, inv)
		}
	}

	if len(found) == 0 {
		t.Error("the Examples block contains no stressy invocation")
	}

	return found
}

// shell reads documented shell a line at a time, carrying earlier `export`s.
type shell struct{ env []string }

// line returns the invocation a line describes, if it describes one.
func (sh *shell) line(t *testing.T, source, line string) (invocation, bool) {
	t.Helper()
	fields := strings.Fields(line)

	// A console block writes its commands as `$ stressy …`.
	if len(fields) > 0 && fields[0] == "$" {
		fields = fields[1:]
	}
	// A trailing comment is not an argument: `stressy -w 4  # four workers`
	// would otherwise run with `#` and `four` in its command line.
	if i := slices.Index(fields, "#"); i >= 0 {
		fields = fields[:i]
	}
	if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
		return invocation{}, false
	}
	if fields[0] == "export" {
		sh.env = append(sh.env, fields[1:]...)

		return invocation{}, false
	}

	// The `NAME=value cmd` form, which is how --help shows the environment.
	var prefix []string
	for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
		prefix, fields = append(prefix, fields[0]), fields[1:]
	}

	switch {
	case len(fields) > 0 && (fields[0] == "stressy" || fields[0] == "./stressy"):
		return invocation{
			source: source,
			env:    append(slices.Clone(sh.env), prefix...),
			args:   fields[1:],
		}, true

	case len(fields) > 1 && fields[0] == "docker" && fields[1] == "run":
		return dockerRun(t, source, fields)
	}

	return invocation{}, false
}

// dockerRun splits a `docker run` line at the image reference: what follows is
// the container's arguments, `-e NAME=value` before it its environment.
func dockerRun(t *testing.T, source string, fields []string) (invocation, bool) {
	t.Helper()
	for i, field := range fields {
		if !strings.HasPrefix(field, imageRepo) {
			continue
		}
		var env []string
		for j := 2; j < i-1; j++ {
			if fields[j] == "-e" || fields[j] == "--env" {
				env = append(env, fields[j+1])
			}
		}

		return invocation{source: source, env: env, args: fields[i+1:], containerised: true}, true
	}

	// Not skipped quietly: silence is how #28 survived the life of the project.
	t.Errorf("%s: `docker run` names no %s image, so this test cannot tell what it runs", source, imageRepo)

	return invocation{}, false
}

// collapse returns text as one line, so a sentence straddling a wrap matches.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// flowList reads the `["-t", "60s"]` form the Job manifest uses for args.
func flowList(t *testing.T, source, list string) []string {
	t.Helper()
	list = strings.TrimSpace(list)
	if !strings.HasPrefix(list, "[") || !strings.HasSuffix(list, "]") {
		t.Fatalf(`%s: %s is not the flow form ["a", "b"] this test reads`, source, list)
	}
	var args []string
	for item := range strings.SplitSeq(strings.Trim(list, "[]"), ",") {
		args = append(args, strings.Trim(strings.TrimSpace(item), `"`))
	}

	return args
}

// documentedImages returns every image reference the README or `--help` names.
func documentedImages(t *testing.T) []string {
	t.Helper()
	doc := strings.Join(lines(t, readmePath), "\n") + "\n" + cli.Examples
	found := imageRef.FindAllString(doc, -1)
	if found == nil {
		t.Fatalf("%s names no %s image, so there is nothing to check against %s", readmePath, imageRepo, releaseConfigPath)
	}

	return found
}

// withoutComment strips a YAML comment: prose is not configuration.
func withoutComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 && (i == 0 || line[i-1] == ' ') {
		return line[:i]
	}

	return line
}

// publishedImages returns the literal image references a release publishes.
func publishedImages(t *testing.T) []string {
	t.Helper()
	var code []string
	for _, line := range lines(t, releaseConfigPath) {
		code = append(code, withoutComment(line))
	}
	config := strings.Join(code, "\n")
	for _, guard := range []string{prereleaseGuard, guardEnd} {
		config = strings.ReplaceAll(config, guard, "")
	}
	published := imageRef.FindAllString(config, -1)
	if published == nil {
		t.Fatalf("%s publishes no literal %s tag, so the README's pull instructions cannot be checked", releaseConfigPath, imageRepo)
	}
	slices.Sort(published)

	return slices.Compact(published)
}
