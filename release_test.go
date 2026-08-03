package main

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// readmeGOOS and readmeGOARCH map the names README.md advertises to the values
// .goreleaser.yaml builds for. A name the README grows without an entry here
// fails the test rather than being quietly skipped, which is the failure this
// whole file exists to prevent.
var (
	readmeGOOS = map[string]string{
		"Linux":   "linux",
		"macOS":   "darwin",
		"Windows": "windows",
		"FreeBSD": "freebsd",
		"NetBSD":  "netbsd",
		"OpenBSD": "openbsd",
	}

	readmeGOARCH = map[string]string{
		"AMD64": "amd64",
		"ARM64": "arm64",
	}
)

// The two feature bullets that, read together, promise a cross product.
var (
	platformBullet = regexp.MustCompile(`(?m)^- Cross-platform support \(([^)]+)\)`)
	archBullet     = regexp.MustCompile(`(?m)^- Multi-architecture support \(([^)]+)\)`)
)

// TestReleaseMatrixMatchesREADME checks the support matrix the README
// advertises against the targets a release actually builds. The two disagreed
// for the whole life of the project: the README listed six operating systems
// and two architectures on consecutive bullets, .goreleaser.yaml carried an
// `ignore:` block dropping windows/arm64, and nothing tied the files together,
// so no stressy_Windows_arm64 was ever published (#28).
//
// A missing artefact announces itself to nobody — the release is green and the
// download page simply has one fewer row — which is what makes this worth a
// test rather than a careful reading. The one who finds out is the
// Windows-on-ARM user, either by finding no binary or by running the amd64 one
// under emulation, where the measurements a CPU stress tool exists to produce
// are meaningless.
func TestReleaseMatrixMatchesREADME(t *testing.T) {
	const (
		config = ".goreleaser.yaml"
		readme = "README.md"
	)

	cfg := lines(t, config)

	// `ignore:` is the shape #28 took: it removes a pair from the cross product
	// the README describes, while the README states no exception. builds is the
	// only block in this config that takes the key.
	for i, line := range cfg {
		if strings.TrimSpace(line) == "ignore:" {
			t.Errorf("%s:%d has an `ignore:` block, which drops a target out of the matrix %s advertises (#28); either delete it, or name the exception in the README and teach this test about it", config, i+1, readme)
		}
	}

	doc, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v, want the README", readme, err)
	}

	tests := []struct {
		key    string
		bullet *regexp.Regexp
		names  map[string]string
	}{
		{key: "goos", bullet: platformBullet, names: readmeGOOS},
		{key: "goarch", bullet: archBullet, names: readmeGOARCH},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := yamlList(t, config, cfg, tt.key)
			want := advertised(t, readme, string(doc), tt.bullet, tt.names)

			slices.Sort(got)
			slices.Sort(want)

			if !slices.Equal(got, want) {
				t.Errorf("%s builds %s %v, %s advertises %v", config, tt.key, got, readme, want)
			}
		})
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

// yamlList returns the items of the first `key:` list — the six entries under
// `goos:`, say. It is not a YAML parser: it knows the one shape this config
// uses, a key alone on its line followed by `- value` items, and it matches the
// key alone so that the `goos: linux` scalars in the `dockers` blocks are not
// mistaken for the build matrix. Pulling in a YAML library for one test would
// be an odd thing to do in a repository that has spent two releases removing
// its config-parsing dependencies (#18, #20).
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

	t.Fatalf("%s has no `%s:` list, so the release builds nothing this test can check against %s", name, key, "README.md")

	return nil
}

// advertised turns the parenthesised list in a README bullet into the GOOS or
// GOARCH values it names. Read out of the README's own words rather than
// duplicated as a literal here: a bullet edited to promise one more platform
// has to be what fails.
func advertised(t *testing.T, name, doc string, bullet *regexp.Regexp, known map[string]string) []string {
	t.Helper()

	match := bullet.FindStringSubmatch(doc)
	if match == nil {
		t.Fatalf("%s has no bullet matching %s, so there is no advertised matrix to check the release against", name, bullet)
	}

	var values []string
	for item := range strings.SplitSeq(match[1], ",") {
		item = strings.TrimSpace(item)

		value, ok := known[item]
		if !ok {
			t.Fatalf("%s advertises %q, which this test cannot map to a GOOS or GOARCH value: add it to the table in this file, and to %s", name, item, ".goreleaser.yaml")
		}

		values = append(values, value)
	}

	return values
}
