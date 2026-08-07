# Contributing

Thanks for taking an interest. stressy is small on purpose — one command, three
flags, three direct dependencies — so most of what follows is about keeping it
that way.

## Getting set up

You need Go at the version in [`go.mod`](go.mod) or newer. CI resolves
`stable`, which is the newest Go release; the toolchain versions CI and the
release pipeline use live in [`.github/versions.env`](.github/versions.env),
which is the one file to edit if a tool needs pinning.

```bash
git clone https://github.com/felipeneuwald/stressy.git
cd stressy
go build
./stressy -w 2 -t 5s
```

## The checks

CI runs these on every push to main and every pull request, and a red one
blocks the merge — a push to a topic branch with no pull request open runs
nothing. Running them locally first is faster than finding out from the pull
request:

```bash
golangci-lint fmt --diff   # formatting, as a patch you can read
golangci-lint run ./...    # lint
go vet ./...               # not redundant: golangci-lint's uniq-by-line drops
                           # a vet finding that shares a line with another
go mod tidy -diff          # the only thing keeping go.mod tidy
go mod verify
go build ./...
go test -race ./...
```

`golangci-lint` is pinned in `.github/versions.env`; install that version rather
than the newest, or you may see findings CI will not. CI verifies the tarball it
downloads against the SHA-256 pinned beside that version, so bumping one line
there means bumping both.

`Build and test` runs on `ubuntu-latest`, `macos-latest` and `windows-latest`. A
release ships binaries for all three, and until #66 `go test` had only ever run
on the first — the release dry run's twelve cross-compiles say the code builds
everywhere, not that it runs anywhere. Lint, the vulnerability scan and the
release dry run stay Linux-only, since their findings do not vary by runner. The
two `//go:build unix` files stay out of the Windows build by design: signalling
your own process is not something Windows supports, so the shutdown paths that
need it are covered on the other two.

Two further jobs need no local equivalent in the normal case. `govulncheck`
scans the dependency graph, and a release dry run (`goreleaser check` and
`goreleaser release --snapshot --clean`) rehearses all twelve cross-compiles and
both image builds. The dry run exists because `.github/workflows/goreleaser.yml`
only fires on a `v*` tag, so before it a change to `.goreleaser.yaml` or the
`Dockerfile` was validated for the first time at the moment it published. If you
touch either of those files, expect that job to be the one that fails.

## Tests

Test what an operator can observe: the flags, the environment variables, the
messages, the exit behaviour. `internal/stressy` is covered by tests that run
the real goroutines, which is what `-race` in CI is there for.

There is also a house rule worth knowing before you write documentation:

> A claim the documentation makes about something else in the repository gets a
> test holding the two together.

This is not decoration. The README advertised six operating systems and two
architectures for the whole life of the project while `.goreleaser.yaml` quietly
excluded `windows/arm64`, and nobody found out, because a missing release
artefact announces itself to no one — the release is green and the download list
is one row shorter. So:

- `release_test.go` holds `.goreleaser.yaml`'s build matrix to the whole cross
  product a release is meant to publish, and fails if an `ignore:` block
  reappears.
- `docs_test.go` runs every stressy invocation in the README and in `--help`
  through the real command, so an example naming a flag that no longer exists is
  a failing build; it also checks that every image the README tells people to
  pull is one a release actually publishes, and that the containerised examples
  are bounded.
- `version_test.go` checks the `-X` target in `.goreleaser.yaml` against the
  variable in `version.go`, because a `-X` naming a variable that does not exist
  is dropped by the linker rather than failing the link — the two files
  disagreeing would have been visible only in a published release.

None of these needed a YAML or Markdown library. They know the one shape the
file they read actually uses, which is the right trade in a repository that has
spent two releases removing its config-parsing dependencies.

## Commits, branches and pull requests

Branches are named `type/short-description`: `fix/`, `feat/`, `docs/`, `deps/`,
`refactor/`, `ci/`, `release/`.

Commit subjects use the same prefixes and say what the change does, in the
imperative and in lower case:

```
fix: publish the windows/arm64 binary the README promises
deps: drop viper, reading the environment directly instead
```

`docs:`, `test:` and `ci:` are filtered out of generated release notes by
`.goreleaser.yaml`, so use them for changes that a user of the binary would not
notice, and one of the others for changes they would.

One issue per pull request where you can. The pull request is where the
reasoning goes — what you tried, what it cost, what you decided against — and
it is what someone reads a year later when they want to know why.

## The changelog

Every change a user could notice gets an entry under `[Unreleased]` in
[`CHANGELOG.md`](CHANGELOG.md), following [Keep a
Changelog](https://keepachangelog.com/en/1.0.0/): `Added`, `Changed`,
`Deprecated`, `Removed`, `Fixed`, `Security`.

One line per entry, in the imperative present tense, under about fifteen words,
saying only what a user of the binary or image can observe. The reasoning and
the issue number belong in the pull request, not here. A change only the tests,
CI, comments, internal layout or documentation can see gets no entry at all.

## Releasing

For maintainers:

1. Move the `[Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD`
   heading. Check that what it claims is what ships — the 0.3.2 entry's
   security claim did not, and that took eighteen months to notice.
2. Tag `vX.Y.Z` and push the tag. `.github/workflows/goreleaser.yml` gates on
   CI first: every job in `ci.yml` must be green on the tagged commit, and a red
   one, one still running, and a commit CI never ran for all stop the release
   before anything is published (#68). Past the gate it builds the twelve
   binaries, both images and the multi-arch manifest, and publishes them. If the
   gate stops a tag, fix what is red and re-run the workflow — the tag does not
   need pushing again.
3. To rehearse the publishing half — the part `--snapshot` cannot cover, since
   it skips the registry login, the image push and the release creation — tag a
   `vX.Y.Z-rcN` pre-release first. `prerelease: auto` and the
   `{{ if not .Prerelease }}` guards keep it off `:latest` and off GitHub's
   "Latest release".

## Security

Do not open a public issue for a vulnerability. [SECURITY.md](SECURITY.md) has
the reporting channel and what is in scope.
