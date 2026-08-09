package main

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const releaseWorkflowPath = ".github/workflows/goreleaser.yml"

// TestReleaseMatrixIsComplete checks the targets a release builds against the
// whole cross product it is meant to publish. An `ignore:` block dropped
// windows/arm64 for the life of the project, and nothing failed (#28).
func TestReleaseMatrixIsComplete(t *testing.T) {
	const config = ".goreleaser.yaml"

	cfg := lines(t, config)

	// `ignore:` is the shape #28 took; builds is the only block taking the key.
	for i, line := range cfg {
		if strings.TrimSpace(line) == "ignore:" {
			t.Errorf("%s:%d has an `ignore:` block, which drops a target out of the release matrix (#28); either delete it, or teach this test about the exception", config, i+1)
		}
	}

	tests := []struct {
		key  string
		want []string
	}{
		{key: "goos", want: []string{"linux", "windows", "darwin", "freebsd"}},
		{key: "goarch", want: []string{"amd64", "arm64"}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := yamlList(t, config, cfg, tt.key)

			slices.Sort(got)
			slices.Sort(tt.want)

			if !slices.Equal(got, tt.want) {
				t.Errorf("%s builds %s %v, want %v", config, tt.key, got, tt.want)
			}
		})
	}
}

// ldflagsVar matches the variable .goreleaser.yaml stamps, capturing the name
// out of `-s -w -X main.injected={{.Version}}`.
var ldflagsVar = regexp.MustCompile(`-X main\.(\w+)=`)

// TestLdflagsVariableMatchesGoreleaser pins the one failure in this area that
// cannot announce itself. `-X main.whatever=` naming a variable that does not
// exist is not a link error — the value is dropped, and the binary quietly
// reports the build-info fallback instead of the version it was released as.
// Renaming the variable on either side, or deleting it, therefore has to fail
// here or it fails in a published release.
func TestLdflagsVariableMatchesGoreleaser(t *testing.T) {
	const config = ".goreleaser.yaml"

	cfg, err := os.ReadFile(config)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the release config", config, err)
	}

	match := ldflagsVar.FindSubmatch(cfg)
	if match == nil {
		t.Fatalf("%s has no `-X main.<var>=` in its ldflags; releases would report the build-info fallback", config)
	}

	// Matched against the source rather than against a literal here: a rename
	// that updates only one of the two files has to be what fails.
	name := string(match[1])
	decl := regexp.MustCompile(`(?m)^var ` + regexp.QuoteMeta(name) + ` string$`)

	const source = "main.go"

	src, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the version source", source, err)
	}

	if !decl.Match(src) {
		t.Errorf("%s stamps main.%s, which %s does not declare as `var %s string`", config, name, source, name)
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

// lines reads a file in the repository root, which is where `go test` runs.
func lines(t *testing.T, name string) []string {
	t.Helper()

	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the file", name, err)
	}

	return strings.Split(string(b), "\n")
}

// yamlList returns the items of the first `key:` list. Not a YAML parser: it
// knows the one shape this config uses, so the `goos: linux` scalars in the
// `dockers` blocks are not mistaken for the build matrix (#18, #20).
func yamlList(t *testing.T, name string, src []string, key string) []string {
	t.Helper()

	for i, line := range src {
		if strings.TrimSpace(line) != key+":" {
			continue
		}

		var items []string
		for _, item := range src[i+1:] {
			item = strings.TrimSpace(item)
			if !strings.HasPrefix(item, "- ") {
				break
			}

			items = append(items, strings.TrimSpace(strings.TrimPrefix(item, "- ")))
		}

		if items == nil {
			t.Fatalf("%s:%d has `%s:` with nothing under it", name, i+1, key)
		}

		return items
	}

	t.Fatalf("%s has no `%s:` list, so the release builds nothing this test can check", name, key)

	return nil
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
