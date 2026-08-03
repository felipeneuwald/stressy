package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

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
	readmePath        = "README.md"
	releaseConfigPath = ".goreleaser.yaml"

	// imageRepo is this project's published image, without a tag. A reference
	// starting with it is stressy's own and gets checked; anything else in a
	// documented command line belongs to somebody else.
	imageRepo = "ghcr.io/felipeneuwald/stressy"
)

// imageRef matches a tagged reference to imageRepo. The tag pattern deliberately
// admits no `{`, so the templated `:{{ .Version }}-amd64` entries in the release
// config do not match: they name no literal tag, and no documentation can refer
// to one.
var imageRef = regexp.MustCompile(regexp.QuoteMeta(imageRepo) + `:[\w.\-]+`)

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

// run executes the invocation against the real command, with the stress test
// itself stubbed out by newTestCmd, and returns what the run was configured
// with.
func (inv invocation) run(t *testing.T) stressy.Cfg {
	t.Helper()

	// The ambient environment configures a run too, so a developer with
	// STRESSY_WORKERS exported would otherwise be testing something other than
	// what the documentation says. Empty rather than unset because that is the
	// behaviour bindEnv documents and keeps deliberately: `STRESSY_WORKERS=${WORKERS}`
	// with WORKERS undefined is a common shape in compose files and pod specs.
	t.Setenv("STRESSY_WORKERS", "")
	t.Setenv("STRESSY_TIMEOUT", "")

	for _, assignment := range inv.env {
		name, value, ok := strings.Cut(assignment, "=")
		if !ok {
			t.Fatalf("%s: %q is not a NAME=value assignment", inv.source, assignment)
		}

		t.Setenv(name, value)
	}

	var cfg stressy.Cfg
	cmd := newTestCmd(t, &cfg)
	cmd.SetArgs(inv.args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("%s: documented invocation `%s` = %v, want it to work as written", inv.source, strings.Join(append(slices.Clone(inv.env), append([]string{"stressy"}, inv.args...)...), " "), err)
	}

	return cfg
}

// documentedInvocations gathers every stressy run the project publishes.
func documentedInvocations(t *testing.T) []invocation {
	t.Helper()

	found := append(readmeInvocations(t), exampleInvocations(t)...)
	if len(found) == 0 {
		t.Fatal("no documented invocations found, so these tests are checking nothing")
	}

	return found
}

// readmeInvocations reads the README's fenced blocks: shell for the command
// lines, YAML for the Kubernetes Job.
func readmeInvocations(t *testing.T) []invocation {
	t.Helper()

	var (
		found   []invocation
		inBlock bool
		lang    string
		sh      shell
		image   string
	)

	for i, line := range lines(t, readmePath) {
		trimmed := strings.TrimSpace(line)
		source := fmt.Sprintf("%s:%d", readmePath, i+1)

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
		case "bash":
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

// publishedImages returns the literal image references a release publishes. The
// `{{ if not .Prerelease }}` guards are stripped first — they keep the floating
// tags off a release candidate and say nothing about whether the tag exists.
func publishedImages(t *testing.T) []string {
	t.Helper()

	config := strings.Join(lines(t, releaseConfigPath), "\n")
	for _, guard := range []string{"{{ if not .Prerelease }}", "{{ end }}"} {
		config = strings.ReplaceAll(config, guard, "")
	}

	published := imageRef.FindAllString(config, -1)
	if published == nil {
		t.Fatalf("%s publishes no literal %s tag, so the README's pull instructions cannot be checked", releaseConfigPath, imageRepo)
	}

	slices.Sort(published)

	return slices.Compact(published)
}
