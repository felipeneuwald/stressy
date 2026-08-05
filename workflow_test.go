package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	releaseWorkflowPath = ".github/workflows/goreleaser.yml"
	versionsPath        = ".github/versions.env"
	loadVersionsPath    = ".github/actions/load-versions/action.yml"
)

// usesRef matches a step's `uses:` and captures both the ref and whatever
// follows it on the line, which for a pinned action is its `# v7.0.1` comment.
var usesRef = regexp.MustCompile(`^\s*(?:- )?uses:\s*(\S+)\s*(.*)$`)

// pinnedRef is `owner/repo` at a full commit SHA, with the optional
// subdirectory an action published out of a monorepo is referenced by.
var pinnedRef = regexp.MustCompile(`^[\w.-]+/[\w.-]+(?:/[\w./-]+)?@[0-9a-f]{40}$`)

// versionComment is the `# v7.0.1` that has to travel with a SHA — the form
// dependabot writes, and the only thing making a pin readable by a human.
var versionComment = regexp.MustCompile(`^#\s*v\d+(?:\.\d+)*`)

// manifestKey matches a `KEY=value` line in .github/versions.env.
var manifestKey = regexp.MustCompile(`^([A-Z0-9_]+)=`)

// jobName matches a job's `name:` in a workflow: four spaces of indentation,
// which is where a key of a job sits and where nothing else does — a step's is
// `      - name:` and the workflow's own is at the left margin.
var jobName = regexp.MustCompile(`^    name: (\S.*)$`)

// requiredJob matches one entry of the gate's `required=(…)` shell array.
var requiredJob = regexp.MustCompile(`^"(.+)"$`)

// testedRunners reads CONTRIBUTING's claim about where the test suite runs,
// anchored on the job's own name so that renaming the job has to touch both.
var testedRunners = regexp.MustCompile("`Build and test` runs on ([^.]+)\\.")

// backticked pulls the code-formatted names out of that sentence.
var backticked = regexp.MustCompile("`([\\w.-]+)`")

// TestWorkflowActionsArePinnedToCommitSHAs covers #65. Every third-party
// `uses:` in this repository used to name a major tag — `actions/checkout@v7`
// and five more — and a major tag is a pointer its owner can move. The
// tj-actions/changed-files compromise moved every version tag of an action onto
// one malicious commit at once, and the refs in goreleaser.yml run with
// contents: write and packages: write: enough to publish trojaned binaries to
// the release page and repoint ghcr.io :latest under this project's name.
//
// Worth a test rather than a careful reading because the failure is invisible
// in the direction it arrives: a hand-edited `@v7` in a future diff reads as
// ordinary, runs green, and is indistinguishable from the pinned form until the
// day the tag moves. The rest of the supply chain here is already tight —
// `go mod verify` re-checking restored module zips, a checksummed golangci-lint
// (#67), reproducible builds — and this was the remaining soft spot.
func TestWorkflowActionsArePinnedToCommitSHAs(t *testing.T) {
	var external int

	for _, path := range []string{ciWorkflowPath, releaseWorkflowPath} {
		for i, line := range lines(t, path) {
			match := usesRef.FindStringSubmatch(line)
			if match == nil {
				continue
			}

			ref, trailing := match[1], strings.TrimSpace(match[2])
			source := fmt.Sprintf("%s:%d", path, i+1)

			// The composite action in this repository is a path ref, resolved
			// from the commit already checked out: no third party, nothing to
			// pin. It is worth a check of its own all the same, since a
			// mistyped path fails at the run rather than in the diff.
			if dir, ok := strings.CutPrefix(ref, "./"); ok {
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					t.Errorf("%s uses %s, which is not a directory in this repository", source, ref)
				}

				continue
			}

			external++

			if !pinnedRef.MatchString(ref) {
				t.Errorf("%s uses %s, a ref whose owner — or whoever compromises them — can repoint it at another commit; pin it to the 40-character SHA that tag resolves to today, with the version as a trailing comment (#65)", source, ref)

				continue
			}

			if !versionComment.MatchString(trailing) {
				t.Errorf("%s pins %s to a SHA with no `# vX.Y.Z` beside it, which leaves nobody able to read what version runs, and dependabot without the version it bumps from (#65)", source, ref)
			}
		}
	}

	// The steady state of this test is finding nothing wrong, so a regexp that
	// quietly stopped matching `uses:` at all would leave it passing forever.
	if external == 0 {
		t.Errorf("neither workflow appears to use a third-party action, which is not true of them; usesRef no longer reads the shape they are written in, so this test is checking nothing")
	}
}

// TestVersionsManifestKeysAreExposed holds .github/versions.env's own header to
// the action beside it: it calls itself the canonical manifest and says every
// key is sourced and exposed as an output. A key added there and forgotten in
// load-versions is a version nobody can consume, and — since the action reads
// each with `:?` — one deleted from the manifest has to fail there, naming
// itself, rather than passing an empty version to a setup action. That guard is
// what the checksum in #67 relies on to never install unverified.
func TestVersionsManifestKeysAreExposed(t *testing.T) {
	action := strings.Join(lines(t, loadVersionsPath), "\n")

	var keys int

	for i, line := range lines(t, versionsPath) {
		match := manifestKey.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		keys++

		if guard := fmt.Sprintf("${%s:?missing from ", match[1]); !strings.Contains(action, guard) {
			t.Errorf("%s:%d declares %s, which %s does not read as %s…} — so it reaches no workflow, and deleting it later would fail somewhere other than here", versionsPath, i+1, match[1], loadVersionsPath, guard)
		}
	}

	if keys == 0 {
		t.Fatalf("%s declares no KEY=value at all, which is not what CI reads its tool versions from", versionsPath)
	}
}

// TestLintInstallVerifiesItsDownload covers #67. The lint job piped the
// golangci-lint release tarball straight from curl into `sudo tar -xz -C
// /usr/local/bin`: the version was pinned, the artefact was trusted blind, and
// a replaced release asset would have executed arbitrary code in CI. It is the
// one binary CI installs outside a checksummed ecosystem — govulncheck arrives
// through `go run` and the module checksum database, setup-go verifies its own
// downloads — and golangci-lint publishes a checksums file with every release
// that went unused.
//
// The regression this catches is the pipe coming back: `curl | tar` is shorter
// than the verified form, is what every install snippet on the internet shows,
// and extracts before anything has been checked.
func TestLintInstallVerifiesItsDownload(t *testing.T) {
	src := lines(t, ciWorkflowPath)
	workflow := strings.Join(src, "\n")

	for _, want := range []string{
		"steps.versions.outputs.golangci-lint-sha256",
		"sha256sum -c",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("%s no longer contains %q, so the golangci-lint tarball is extracted without being checked against the digest in %s (#67)", ciWorkflowPath, want, versionsPath)
		}
	}

	for i, line := range src {
		code := strings.TrimSpace(withoutComment(line))

		if strings.HasPrefix(code, "|") && strings.Contains(code, "tar") {
			t.Errorf("%s:%d pipes a download into tar, which extracts it before anything has verified it — download to a file, `sha256sum -c` it, then extract (#67)", ciWorkflowPath, i+1)
		}
	}
}

// TestReleaseIsGatedOnGreenCI covers #68. Pushing a `v*` tag went straight to
// publishing with contents: write and packages: write, and nothing checked that
// the tagged commit had passed anything: a tag on a red commit, on a commit
// still building, or on one from a branch that was never pushed, uploaded
// twelve binaries, pushed both images and repointed ghcr.io :latest all the
// same. The pipeline is deliberate everywhere else — the release dry run exists
// precisely so release-path changes are validated before a tag — and there is
// no server-side backstop: the repository has no tag ruleset and no protected
// environment.
//
// Both halves are held here, because the gate is only as good as the token
// beside it: an edge that disappears in a future diff publishes ungated, and a
// write grant that drifts back to the workflow level hands it to the gate too.
func TestReleaseIsGatedOnGreenCI(t *testing.T) {
	src := lines(t, releaseWorkflowPath)

	release := jobBlock(t, releaseWorkflowPath, src, "goreleaser")

	if !slices.ContainsFunc(release, func(line string) bool {
		return strings.TrimSpace(withoutComment(line)) == "needs: gate"
	}) {
		t.Errorf("%s: the goreleaser job has no `needs: gate`, so a `v*` tag publishes whatever CI made of the commit, including nothing at all (#68)", releaseWorkflowPath)
	}

	// The gate is the job that must not be able to publish; the goreleaser job
	// is the only one that must. A `write` at the workflow level is granted to
	// both.
	for i, line := range topLevelBlock(t, releaseWorkflowPath, src, "permissions") {
		if strings.Contains(withoutComment(line), ": write") {
			t.Errorf("%s:%d grants a write permission at the workflow level, where it reaches the gate as well; scope it to the goreleaser job (#68)", releaseWorkflowPath, i+1)
		}
	}
}

// TestReleaseGateRequiresEveryCIJob holds the gate's list of jobs to the jobs
// ci.yml defines. The gate names them in a shell array, and a fifth CI job
// added without a line there is ungated in the way that is hardest to notice:
// the release still runs the gate, the gate still passes, and the job nobody
// added to it is the one that was red.
//
// Names are matched exactly here and by prefix in the gate itself, which is
// what lets the Build and test matrix report one check per runner (#66).
func TestReleaseGateRequiresEveryCIJob(t *testing.T) {
	gated := gateRequiredJobs(t)
	defined := ciJobNames(t)

	slices.Sort(gated)
	slices.Sort(defined)

	if !slices.Equal(gated, defined) {
		t.Errorf("%s gates a release on %v, where %s defines the jobs %v; a job in neither list is a job whose result no release waits for (#68)", releaseWorkflowPath, gated, ciWorkflowPath, defined)
	}
}

// TestDocumentedTestMatrixMatchesTheWorkflow covers the documented half of #66.
// A release ships binaries for six operating systems and `go test` ran on one
// of them, which the release dry run's twelve cross-compiles hid: they say the
// code builds everywhere, not that it runs anywhere. CONTRIBUTING now tells a
// contributor which runners their pull request will be judged on, and that
// sentence is the kind that goes stale silently — a runner dropped from the
// matrix produces no failure anywhere, and the document keeps promising a check
// that no longer happens.
func TestDocumentedTestMatrixMatchesTheWorkflow(t *testing.T) {
	claim := testedRunners.FindStringSubmatch(unwrapped(t, contributingPath))
	if claim == nil {
		t.Fatalf("%s no longer says which runners `Build and test` runs on, so nothing holds it to the matrix in %s (#66)", contributingPath, ciWorkflowPath)
	}

	var documented []string
	for _, name := range backticked.FindAllStringSubmatch(claim[1], -1) {
		documented = append(documented, name[1])
	}

	configured := matrixRunners(t)

	slices.Sort(documented)
	slices.Sort(configured)

	if !slices.Equal(documented, configured) {
		t.Errorf("%s says the test suite runs on %v, where %s runs it on %v (#66)", contributingPath, documented, ciWorkflowPath, configured)
	}
}

// matrixRunners returns the runners the Build and test job is matrixed over.
//
// Not a YAML parser, for the reason release_test.go is not one either: it knows
// the one shape this file uses, a single `os:` flow list, and requires it to
// stay the only one rather than silently reading the first of several — a
// second matrix elsewhere in the file is the point at which this needs teaching
// rather than guessing.
func matrixRunners(t *testing.T) []string {
	t.Helper()

	var found []string

	for i, line := range lines(t, ciWorkflowPath) {
		code := strings.TrimSpace(withoutComment(line))

		if list, ok := strings.CutPrefix(code, "os:"); ok {
			found = append(found, flowList(t, fmt.Sprintf("%s:%d", ciWorkflowPath, i+1), list)...)
		}
	}

	if found == nil {
		t.Fatalf("%s has no `os:` matrix, so the test suite runs on one runner and %s cannot be claiming three (#66)", ciWorkflowPath, contributingPath)
	}

	return found
}

// gateRequiredJobs returns the CI jobs the release gate insists are green,
// read out of the `required=(…)` array in its shell step.
func gateRequiredJobs(t *testing.T) []string {
	t.Helper()

	src := lines(t, releaseWorkflowPath)

	start := slices.IndexFunc(src, func(line string) bool {
		return strings.TrimSpace(line) == "required=("
	})
	if start < 0 {
		t.Fatalf("%s has no `required=(` array, so nothing says which jobs the gate holds a release to (#68)", releaseWorkflowPath)
	}

	var jobs []string

	for _, line := range src[start+1:] {
		item := strings.TrimSpace(line)
		if item == ")" {
			return jobs
		}

		match := requiredJob.FindStringSubmatch(item)
		if match == nil {
			t.Fatalf("%s: %q is not one of the quoted job names this array is written in", releaseWorkflowPath, item)
		}

		jobs = append(jobs, match[1])
	}

	t.Fatalf("%s: the `required=(` array is never closed", releaseWorkflowPath)

	return nil
}

// ciJobNames returns the name every job in the CI workflow reports itself by —
// which is what a check run is called, and so what the gate matches against.
func ciJobNames(t *testing.T) []string {
	t.Helper()

	var names []string

	for _, line := range lines(t, ciWorkflowPath) {
		if match := jobName.FindStringSubmatch(line); match != nil {
			names = append(names, strings.TrimSpace(match[1]))
		}
	}

	if names == nil {
		t.Fatalf("%s defines no job with a `name:`, so its jobs report themselves by their key and nothing in the gate would match them", ciWorkflowPath)
	}

	return names
}

// jobBlock returns the lines of one job, by the key it is declared under: from
// `  goreleaser:` to the next key at that indentation. Scoped rather than
// searched whole-file so that a setting belonging to the gate is never read as
// the release's, which is the mistake this whole file exists to make
// impossible.
func jobBlock(t *testing.T, name string, src []string, job string) []string {
	t.Helper()

	return block(t, name, src, "  "+job+":", "    ")
}

// topLevelBlock returns the lines under a key at the left margin — the
// workflow's own `permissions:`, as against a job's.
func topLevelBlock(t *testing.T, name string, src []string, key string) []string {
	t.Helper()

	return block(t, name, src, key+":", " ")
}

// block returns a file's lines with everything outside one indented block
// blanked out, so that an index into the result is still the line number in the
// file — the shape unreleased uses in docs_test.go, and for the same reason: a
// finding has to be able to name where it is.
func block(t *testing.T, name string, src []string, key, indent string) []string {
	t.Helper()

	start := slices.Index(src, key)
	if start < 0 {
		t.Fatalf("%s has no `%s` block", name, key)
	}

	scoped := make([]string, len(src))

	for i := start + 1; i < len(src); i++ {
		code := withoutComment(src[i])

		if strings.TrimSpace(code) == "" {
			continue
		}

		if !strings.HasPrefix(code, indent) {
			break
		}

		scoped[i] = src[i]
	}

	return scoped
}
