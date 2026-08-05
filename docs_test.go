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

	"github.com/spf13/pflag"

	"github.com/felipeneuwald/stressy/internal/stressy"
)

// Documentation drifts away from the thing it documents silently, which is what
// makes it worth a test rather than a careful reading: nobody re-runs the
// README, and a command line that stopped working keeps sitting there looking
// correct. These tests run every stressy invocation the project publishes —
// the README's code blocks and the Example block cobra prints under `--help` —
// through the real command, and check the two claims those examples make about
// the world beyond themselves: that the images they name are published, and
// that the containerised ones stop on their own (#27a, #27b, #27c).
//
// Same approach as TestReleaseMatrixMatchesREADME in release_test.go, and the
// same trade in the parsing: these know the one shape the documentation
// actually uses and fail on anything else, rather than pulling a Markdown or
// YAML library into a repository that has spent two releases removing its
// config-parsing dependencies (#18, #20).

const (
	readmePath = "README.md"
	// CONTRIBUTING.md publishes an invocation of its own — `./stressy -w 2 -t
	// 5s`, the first thing a new contributor runs — in the one document that
	// states the house rule requiring these tests (#59).
	contributingPath = "CONTRIBUTING.md"
	// The Dockerfile is documentation's subject rather than documentation: the
	// README's Kubernetes guidance is built on the UID it sets (#55).
	dockerfilePath    = "Dockerfile"
	releaseConfigPath = ".goreleaser.yaml"

	// imageRepo is this project's published image, without a tag. A reference
	// starting with it is stressy's own and gets checked; anything else in a
	// documented command line belongs to somebody else.
	imageRepo = "ghcr.io/felipeneuwald/stressy"

	// stopHint opens the line an indefinite run prints where a bounded one
	// prints nothing, and helpPointer closes it. The README shows the whole
	// line in its sample session and TestExitCodes matches a real child
	// process's output against these same strings, so the documented output and
	// the printed one cannot drift apart — which matters more here than
	// anywhere else in this file, because the reader who needs the stop
	// instruction is the one who has not read the README (#52).
	stopHint    = "Press Ctrl+C or send SIGTERM to stop."
	helpPointer = "Use --help for additional information"

	// prereleaseGuard and guardEnd are what wrap a floating image tag in the
	// release config. goreleaser skips an image or manifest whose template
	// renders empty, so a tag inside them is published by a full release and by
	// nothing else — which is the whole of what keeps `:latest` off a release
	// candidate (#23, #56).
	prereleaseGuard = "{{ if not .Prerelease }}"
	guardEnd        = "{{ end }}"
)

// imageRef matches a tagged reference to imageRepo. The tag pattern deliberately
// admits no `{`, so the templated `:{{ .Version }}-amd64` entries in the release
// config do not match: they name no literal tag, and no documentation can refer
// to one.
var imageRef = regexp.MustCompile(regexp.QuoteMeta(imageRepo) + `:[\w.\-]+`)

// summaryLine is the shape of the line a finished run prints (#49). The README
// shows one in its sample session and TestExitCodes matches a real child
// process's output against this same pattern, so the documented output and the
// printed output cannot drift apart — the failure this whole file exists to
// prevent, in the one place the documentation quotes stressy back to itself.
var summaryLine = regexp.MustCompile(`^Computed (\d+) hash(?:es)? in \S+ \(\d+\.\d+ hashes/s, \d+ workers?\)$`)

// exitCodeRow matches a row of the README's exit-code table: the code in
// backticks in the first cell, what it means in the second. It knows the one
// shape that table uses, like everything else in this file.
var exitCodeRow = regexp.MustCompile("(?m)^\\|\\s*`(\\d+)`\\s*\\|\\s*(.+?)\\s*\\|\\s*$")

// documentedFlag matches a bullet of the README's Available Flags list,
// capturing the shorthand and the long name out of the `-w, --workers` code
// span that opens it.
var documentedFlag = regexp.MustCompile("(?m)^- `-(\\w), --([\\w-]+)`:")

// dockerUser matches the Dockerfile's numeric `USER uid:gid`, and documentedUID
// the claim the README makes about it: the phrase UID/GID `65532`, wherever it
// appears.
var (
	dockerUser    = regexp.MustCompile(`(?m)^USER (\d+):(\d+)\s*$`)
	documentedUID = regexp.MustCompile("UID/GID `(\\d+)`")
)

// readmeSignalNames maps a signal a run stops on to the name the README has to
// call it by. A signal added to stressy.ShutdownSignals without an entry here
// fails the test rather than being quietly skipped, the same trade
// release_test.go's readmeGOOS makes.
var readmeSignalNames = map[os.Signal]string{
	syscall.SIGINT:  "SIGINT",
	syscall.SIGTERM: "SIGTERM",
}

// invocation is a stressy run lifted out of the documentation: where it was
// written, the environment it is prefixed with, the flags it passes, and
// whether it runs in a container — where nobody is watching a terminal, and an
// unbounded run is a container loading every CPU it can reach until somebody
// notices (#27b).
type invocation struct {
	source        string
	env           []string
	args          []string
	containerised bool
}

// TestDocumentedInvocationsAreValid runs every documented command line through
// the command that has to accept it. It is the cheap half of keeping `--help`
// and the README honest: an example naming a flag that no longer exists, or
// passing a value the flag no longer parses, fails the build rather than the
// reader.
func TestDocumentedInvocationsAreValid(t *testing.T) {
	for _, inv := range documentedInvocations(t) {
		t.Run(inv.source, func(t *testing.T) {
			inv.run(t)
		})
	}
}

// TestContainerExamplesAreBounded covers #27b. The README's first Docker
// example used to be `docker run ghcr.io/felipeneuwald/stressy:latest` with no
// timeout and no word about how to stop it, and the same omission in a
// Kubernetes Job is worse than untidy: a container that never terminates leaves
// the Job at 0/1 forever, so the run never completes and never cleans up.
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

// TestDocumentedImagesAreReleased checks every image the documentation tells
// someone to pull against the ones a release actually publishes. The README
// advertises three tags — the multi-arch `:latest` and the two per-architecture
// ones — and nothing but this test connects them to the config that produces
// them, so dropping a tag from the release would leave the README advertising a
// pull that 404s.
func TestDocumentedImagesAreReleased(t *testing.T) {
	published := publishedImages(t)

	for _, ref := range documentedImages(t) {
		if !slices.Contains(published, ref) {
			t.Errorf("documentation names %s, which %s does not publish; it publishes %v", ref, releaseConfigPath, published)
		}
	}
}

// TestDocumentedExitCodes checks the README's exit-code table against the codes
// a run actually produces. #48 is the reason there is a table at all: a
// signal-interrupted run used to exit 0, so there was nothing to document, and
// the Kubernetes section's "waits for it to exit 0 and records the run as
// finished" quietly recorded an evicted pod as a completed one.
//
// The two directions are both worth checking. A signal handled here and
// documented nowhere leaves an operator reading 143 out of `kubectl get pod`
// with nothing to look it up in; a code documented here and produced by nothing
// sends them looking for a failure mode that does not exist.
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

	// The two codes that predate #48 and did not change: a completed run and a
	// rejected configuration.
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

// TestDocumentedRunOutput checks the sample session in the README against the
// line a run prints. The summary line is new in #49 and carries measured
// numbers, so what a sample can be held to is its shape — which is exactly what
// drifts: a format changed in the code and left alone in the README goes on
// looking correct, and it is the line an operator is told to compare across
// nodes.
func TestDocumentedRunOutput(t *testing.T) {
	var found int

	for i, line := range lines(t, readmePath) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Computed ") {
			continue
		}

		found++

		if !summaryLine.MatchString(trimmed) {
			t.Errorf("%s:%d: %q is not the summary line a run prints (#49)", readmePath, i+1, trimmed)
		}
	}

	if found == 0 {
		t.Errorf("%s shows no end-of-run summary line, so nothing holds the documented output to the printed one (#49)", readmePath)
	}
}

// TestDocumentedStopHint covers the documentation half of #52. The `--help`
// pointer used to print on every run, so the README's sample session showed it
// under a bounded one; it now prints only where it can help, beside the
// instruction that says how to stop the run it is printed for.
//
// Both directions are worth holding. A README that shows the pointer on its own
// is quoting output no run produces any more, and a README that shows no stop
// instruction at all has stopped documenting the one thing an indefinite run's
// reader is looking for.
func TestDocumentedStopHint(t *testing.T) {
	var found int

	for i, line := range lines(t, readmePath) {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, helpPointer) {
			continue
		}

		found++

		if !strings.HasPrefix(trimmed, stopHint) {
			t.Errorf("%s:%d: %q shows the --help pointer without the stop instruction that now precedes it; a run prints the two together or neither (#52)", readmePath, i+1, trimmed)
		}
	}

	if found == 0 {
		t.Errorf("%s shows no sample of an indefinite run's second line, so nothing holds the documented stop instruction to the printed one (#52)", readmePath)
	}
}

// TestDocumentedFlagsExist holds the README's Available Flags list to the
// command that has to answer it. Each bullet is two claims — a long name and a
// shorthand — and the flag list is a bullet list rather than a fenced block, so
// it escapes the invocation net above and was checked by nothing.
//
// `-v, --version` is the claim that makes the README side worth pinning. The
// flag itself is held from the command side already — #47's
// TestFlagsCobraSetItselfWorkOnTheCommandLine runs both spellings through the
// command, and deleting the `Version` line newCmd's flag hangs on fails it —
// but nothing read the list. A bullet naming a flag the command does not have,
// a shorthand the command spells differently, and a flag the command grows that
// the list never mentions all passed (#54).
//
// The shorthand is worth pinning from here too: cobra gives --version the `v`
// spelling only while no other flag has claimed it, and registers the flag
// without a shorthand if one has — so a future --verbose repoints what this
// bullet promises without removing anything.
func TestDocumentedFlagsExist(t *testing.T) {
	rows := documentedFlag.FindAllStringSubmatch(strings.Join(lines(t, readmePath), "\n"), -1)
	if rows == nil {
		t.Fatalf("%s lists no flags in the `-w, --workers` bullet shape, so nothing holds its flag list to the command", readmePath)
	}

	// --help and --version are cobra's rather than newCmd's, and cobra
	// registers them as Execute starts; initialising them here is what brings
	// them within reach of a lookup, and is the same pair of calls Execute
	// makes.
	cmd := newCmd(&stressy.Cfg{})
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	documented := make(map[string]bool, len(rows))

	for _, row := range rows {
		shorthand, name := row[1], row[2]
		documented[name] = true

		t.Run(name, func(t *testing.T) {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Fatalf("%s documents --%s, which the command does not have", readmePath, name)
			}

			if f.Shorthand != shorthand {
				t.Errorf("%s documents -%s for --%s, which the command gives the shorthand %q", readmePath, shorthand, name, f.Shorthand)
			}
		})
	}

	// The other direction, checked for the same reason TestDocumentedExitCodes
	// checks both: a flag the command grows and the README never mentions is
	// one a reader finds only by running `--help` and knowing to look.
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !documented[f.Name] {
			t.Errorf("the command has --%s, which %s's flag list does not document", f.Name, readmePath)
		}
	})
}

// TestImageRunsAsDocumentedUser holds the README's claim about the image's
// identity to the Dockerfile that sets it. The Kubernetes manifest omits
// `runAsUser` on purpose — "the image already runs as UID/GID `65532`" — so
// that a pod under the `restricted` Pod Security Standard needs nothing beyond
// what the standard already asks for, and that omission is correct only while
// the Dockerfile keeps its `USER` line (#55).
//
// Nothing else would notice it going. CI's release dry run builds this
// Dockerfile on every pull request, but building an image and running one are
// different things: an image with no `USER`, or a rooted one, builds exactly as
// cleanly, and the first place it shows up is a kubelet refusing to start the
// container under `runAsNonRoot: true` — in a user's cluster, with a green
// release behind it. The pending `dockers_v2` migration rewrites that
// publishing path, which is the change this is here for.
func TestImageRunsAsDocumentedUser(t *testing.T) {
	set := dockerUser.FindAllStringSubmatch(strings.Join(lines(t, dockerfilePath), "\n"), -1)
	if set == nil {
		t.Fatalf("%s sets no numeric `USER uid:gid`, so the image runs as root and every pod spec under runAsNonRoot has to pin runAsUser itself (#55)", dockerfilePath)
	}

	// The last one, which is the one docker applies. There is one today; reading
	// the first would mean a `USER 0:0` appended below it passed this.
	user := set[len(set)-1]
	uid, gid := user[1], user[2]

	// Numeric and non-zero is the whole of what the kubelet's check needs:
	// scratch carries no /etc/passwd for a name to resolve against, and root is
	// the one identity `runAsNonRoot: true` refuses.
	if uid == "0" || gid == "0" {
		t.Errorf("%s sets USER %s:%s, which the kubelet refuses to start under runAsNonRoot: true", dockerfilePath, uid, gid)
	}

	claims := documentedUID.FindAllStringSubmatch(strings.Join(lines(t, readmePath), "\n"), -1)
	if claims == nil {
		t.Fatalf("%s no longer says what UID the image runs as, which is what its Kubernetes manifest leaves runAsUser out on the strength of (#55)", readmePath)
	}

	// The two files are matched against each other rather than each against a
	// literal here, so that changing the UID in both passes and changing it in
	// either alone fails.
	for _, claim := range claims {
		if claim[1] != uid || claim[1] != gid {
			t.Errorf("%s advertises UID/GID %s, %s sets USER %s:%s", readmePath, claim[1], dockerfilePath, uid, gid)
		}
	}
}

// TestFloatingTagsAreGuardedAgainstPrereleases covers #56. The README promises
// that "`:latest` only ever points at a full release; pre-release tags such as
// `v0.4.0-rc1` publish under their own version tag and leave `:latest` alone",
// and the whole of what keeps that true is the guard wrapping each floating tag
// in the release config — the settled fix of #23, asserted by nothing.
//
// Un-guarding one is invisible by construction. A missing guard does not fail a
// build, it publishes a tag — and CI's release dry run never publishes, never
// renders the `docker_manifests` block at all, and never sees a pre-release
// version pushed for real. Meanwhile CONTRIBUTING.md tells maintainers to
// rehearse the publishing half by tagging a `vX.Y.Z-rcN` on the strength of
// exactly these guards. With one gone, that rehearsal repoints a floating tag —
// `:latest` itself, the one `docker run` resolves by default, if it is the
// manifest's — at a release candidate, and the wrong artefact announces itself
// to nobody.
//
// Anchored on the tags rather than on line positions, so the pending
// `dockers_v2` migration inherits the requirement: a floating tag written into
// whatever replaces these blocks carries the guard or fails here.
func TestFloatingTagsAreGuardedAgainstPrereleases(t *testing.T) {
	var found int

	for i, line := range lines(t, releaseConfigPath) {
		line = withoutComment(line)

		// imageRef admits no `{`, so every reference it matches names a literal
		// tag — and a literal tag in a release config is one that stays where
		// it is put while the version moves underneath it. The versioned tags
		// are templated, and match nothing here.
		for _, ref := range imageRef.FindAllString(line, -1) {
			found++

			guard, end, tag := strings.Index(line, prereleaseGuard), strings.Index(line, guardEnd), strings.Index(line, ref)

			if guard < 0 || tag < guard || end < tag {
				t.Errorf("%s:%d publishes the floating tag %s without wrapping it in %s…%s, so tagging a pre-release would repoint it at a release candidate (#23, #56)", releaseConfigPath, i+1, ref, prereleaseGuard, guardEnd)
			}
		}
	}

	if found == 0 {
		t.Errorf("%s names no literal %s tag, so this test is checking nothing: either the floating tags are gone, or they are written in a shape it cannot see", releaseConfigPath, imageRepo)
	}
}

// run executes the invocation against the real command, with the stress test
// itself stubbed out by newTestCmd, and returns what the run was configured
// with.
func (inv invocation) run(t *testing.T) stressy.Cfg {
	t.Helper()

	// The ambient environment configures a run too, so a developer with
	// STRESSY_WORKERS exported would otherwise be testing something other than
	// what the documentation says. newTestCmd blanks those variables, which is
	// why it is built before the documented ones are applied rather than after.
	// See clearStressyEnv.
	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)

	for _, assignment := range inv.env {
		name, value, ok := strings.Cut(assignment, "=")
		if !ok {
			t.Fatalf("%s: %q is not a NAME=value assignment", inv.source, assignment)
		}

		t.Setenv(name, value)
	}

	cmd.SetArgs(inv.args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("%s: documented invocation `%s` = %v, want it to work as written", inv.source, strings.Join(append(slices.Clone(inv.env), append([]string{"stressy"}, inv.args...)...), " "), err)
	}

	return cfg
}

// documentedInvocations gathers every stressy run the project publishes.
//
// CONTRIBUTING.md is read alongside the README because it publishes one too,
// and a flag rename would leave it broken-but-green — the silent drift this
// file exists to prevent, in the document that states the rule (#59). Its other
// bash lines start with tokens the parser below already skips, and its
// commit-subject block is fenced without a language, so nothing there is read
// as shell.
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

// fileInvocations reads a document's fenced blocks: shell for the command
// lines, YAML for the Kubernetes Job.
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
		// console as well as bash: the sample session showing what a run prints
		// is a documented invocation like any other, and the flags in it are
		// the ones a reader will copy.
		case "bash", "console":
			if inv, ok := sh.line(t, source, trimmed); ok {
				found = append(found, inv)
			}
		case "yaml":
			// The one shape the Job manifest uses: a container's `image:`,
			// then the `args:` it runs with.
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

// exampleInvocations reads the Example block cobra prints under `stressy
// --help`. Before #27c there was none, which left --help strictly less
// informative than the README — and in the container image, where the README is
// not present and there is no shell to read it with, it is the only
// documentation that ships.
func exampleInvocations(t *testing.T) []invocation {
	t.Helper()

	example := newCmd(&stressy.Cfg{}).Example
	if strings.TrimSpace(example) == "" {
		t.Fatal("the root command has no Example block, so `stressy --help` shows no examples (#27c)")
	}

	var (
		found []invocation
		sh    shell
	)

	for i, line := range strings.Split(example, "\n") {
		if inv, ok := sh.line(t, fmt.Sprintf("--help:%d", i+1), strings.TrimSpace(line)); ok {
			found = append(found, inv)
		}
	}

	if len(found) == 0 {
		t.Error("the Example block contains no stressy invocation")
	}

	return found
}

// shell reads a snippet of shell one line at a time, carrying the environment
// `export`ed earlier in the same block — which is how the README's environment
// example is written, and the only reason those values are exercised at all.
type shell struct{ env []string }

// line returns the invocation a line of documented shell describes, if it
// describes one. Comments, blanks and the lines that install or build rather
// than run report false.
func (sh *shell) line(t *testing.T, source, line string) (invocation, bool) {
	t.Helper()

	fields := strings.Fields(line)

	// A console block writes its commands as `$ stressy …` and its output
	// unprefixed, so the prompt comes off here and the output lines fall out
	// below like any other line that runs nothing.
	if len(fields) > 0 && fields[0] == "$" {
		fields = fields[1:]
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

// dockerRun separates a `docker run` line into the flags docker reads and the
// ones stressy does: everything after the image reference is the container's
// arguments, and `-e NAME=value` before it is its environment.
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

	// Not skipped quietly: a `docker run` documented here that runs something
	// other than this image is a line this test cannot check, and silence is
	// how #28 survived for the life of the project.
	t.Errorf("%s: `docker run` names no %s image, so this test cannot tell what it runs", source, imageRepo)

	return invocation{}, false
}

// flowList reads the `["-t", "60s"]` form the Job manifest uses for args.
func flowList(t *testing.T, source, list string) []string {
	t.Helper()

	list = strings.TrimSpace(list)
	if !strings.HasPrefix(list, "[") || !strings.HasSuffix(list, "]") {
		t.Fatalf(`%s: args is %s, which this test only reads in the flow form ["-t", "60s"]`, source, list)
	}

	var args []string
	for item := range strings.SplitSeq(strings.Trim(list, "[]"), ",") {
		args = append(args, strings.Trim(strings.TrimSpace(item), `"`))
	}

	return args
}

// documentedImages returns every reference to this project's image that the
// README or `--help` tells someone to pull or run.
func documentedImages(t *testing.T) []string {
	t.Helper()

	doc := strings.Join(lines(t, readmePath), "\n") + "\n" + newCmd(&stressy.Cfg{}).Example

	found := imageRef.FindAllString(doc, -1)
	if found == nil {
		t.Fatalf("%s names no %s image, so there is nothing to check against %s", readmePath, imageRepo, releaseConfigPath)
	}

	return found
}

// withoutComment returns the configuration on a line, with any comment removed:
// in YAML a `#` opens one at the start of a line or after a space.
//
// Prose about the configuration is not the configuration. The header above the
// `dockers` blocks names `ghcr.io/felipeneuwald/stressy:latest` while
// explaining why the guards are there, and a check reading it would find both a
// floating tag with no guard and a published image that nothing publishes.
func withoutComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 && (i == 0 || line[i-1] == ' ') {
		return line[:i]
	}

	return line
}

// publishedImages returns the literal image references a release publishes. The
// `{{ if not .Prerelease }}` guards are stripped first — they keep the floating
// tags off a release candidate and say nothing about whether the tag exists.
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
