package main

import (
	"os"
	"slices"
	"strings"
	"testing"
)

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
		{key: "goos", want: []string{"linux", "windows", "darwin", "freebsd", "netbsd", "openbsd"}},
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
