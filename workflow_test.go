package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const releaseWorkflowPath = ".github/workflows/goreleaser.yml"

// usesRef captures a step's ref and the `# v7.0.1` comment beside it.
var usesRef = regexp.MustCompile(`^\s*(?:- )?uses:\s*(\S+)\s*(.*)$`)

// pinnedRef is `owner/repo` at a full commit SHA, monorepo subdirectory and all.
var pinnedRef = regexp.MustCompile(`^[\w.-]+/[\w.-]+(?:/[\w./-]+)?@[0-9a-f]{40}$`)

// versionComment is the `# v7.0.1` that has to travel with a SHA.
var versionComment = regexp.MustCompile(`^#\s*v\d+(?:\.\d+)*`)

// TestWorkflowActionsArePinnedToCommitSHAs covers #65. A major tag is a pointer
// its owner can move, the refs in goreleaser.yml run with contents: write and
// packages: write, and a hand-edited `@v7` runs green until the tag moves.
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

			// A path ref: no third party, nothing to pin. Still worth a check,
			// since a mistyped path fails at the run rather than in the diff.
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

// TestNothingPipesADownloadIntoTar covers the regression half of #67: `curl |
// tar` is shorter than any verified form and extracts before anything checks.
func TestNothingPipesADownloadIntoTar(t *testing.T) {
	for i, line := range lines(t, ciWorkflowPath) {
		code := strings.TrimSpace(withoutComment(line))

		if strings.HasPrefix(code, "|") && strings.Contains(code, "tar") {
			t.Errorf("%s:%d pipes a download into tar, which extracts it before anything has verified it; fetch the tool with `go run module@version` instead, where the checksum database vouches for what arrives (#67)", ciWorkflowPath, i+1)
		}
	}
}

// TestReleaseIsGatedOnGreenCI covers #68, both halves: an edge that disappears
// publishes ungated, and a workflow-level write grant reaches the gate too.
func TestReleaseIsGatedOnGreenCI(t *testing.T) {
	src := lines(t, releaseWorkflowPath)

	release := block(t, releaseWorkflowPath, src, "  goreleaser:", "    ")

	if !slices.ContainsFunc(release, func(line string) bool {
		return strings.TrimSpace(withoutComment(line)) == "needs: gate"
	}) {
		t.Errorf("%s: the goreleaser job has no `needs: gate`, so a `v*` tag publishes whatever CI made of the commit, including nothing at all (#68)", releaseWorkflowPath)
	}

	// The gate must not be able to publish; a workflow-level `write` lets it.
	for i, line := range block(t, releaseWorkflowPath, src, "permissions:", " ") {
		if strings.Contains(withoutComment(line), ": write") {
			t.Errorf("%s:%d grants a write permission at the workflow level, where it reaches the gate as well; scope it to the goreleaser job (#68)", releaseWorkflowPath, i+1)
		}
	}
}

// block blanks out everything outside one indented block, so an index is still
// a line number and a gate setting is never read as the release's.
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
