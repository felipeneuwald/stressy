# Contributing

stressy is small on purpose: one command, three flags, one direct dependency.

## Getting set up

You need Go at the version in [`go.mod`](go.mod) or newer; CI resolves `stable`.

```bash
git clone https://github.com/felipeneuwald/stressy.git
cd stressy
go build
./stressy -w 2 -t 5s
```

## The checks

CI runs these on every push to main and every pull request; a red one blocks the
merge:

```bash
golangci-lint fmt --diff   # formatting, as a patch you can read
golangci-lint run ./...    # lint
go mod tidy -diff          # the only thing keeping go.mod tidy
go mod verify
go test -race ./...        # also the only thing compiling every package
```

CI runs `golangci-lint` through
`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`; run
that exact version locally rather than the newest, or you may see findings CI
will not. `Build and test` runs on `ubuntu-latest`, `macos-latest` and `windows-latest`;
the `//go:build unix` files stay out of the Windows build, since signalling your
own process is not a thing there. A `govulncheck` scan and a `goreleaser` dry
run are jobs of their own; neither needs a local equivalent.

## Tests

Test what an operator can observe: the flags, the environment variables, the
messages, the exit behaviour. There is also a house rule:

> A claim the documentation makes about something else in the repository gets a
> test holding the two together.

## Commits, branches and pull requests

Branches are named `type/short-description`: `fix/`, `feat/`, `docs/`, `deps/`,
`refactor/`, `ci/`, `release/`. Commit subjects use the same prefixes, in the
imperative and in lower case — `fix: publish the windows/arm64 binary`. `docs:`,
`test:` and `ci:` are filtered out of the release notes, so use them for changes
a user would not notice. One issue per pull request where you can.

## The changelog

Every change a user could notice gets an entry under `[Unreleased]` in
[`CHANGELOG.md`](CHANGELOG.md), following [Keep a
Changelog](https://keepachangelog.com/en/1.0.0/): `Added`, `Changed`,
`Deprecated`, `Removed`, `Fixed`, `Security`.

One line per entry, in the imperative present tense, under about fifteen words,
saying only what a user of the binary or image can observe. The reasoning and
the issue number belong in the pull request, not here. A change only the tests,
CI, comments, internal layout or documentation can see gets no entry at all.

## Releasing (for maintainers)

1. Move the `[Unreleased]` entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading,
   and check that what it claims is what ships.
2. Tag `vX.Y.Z` and push the tag. `.github/workflows/goreleaser.yml` gates on CI
   — every run of `ci.yml` on the tagged commit must have completed successfully
   — then builds the twelve binaries, both images and the multi-arch manifest.
   If the gate stops a tag, fix what is red and re-run the workflow.
3. To rehearse the publishing half, tag a `vX.Y.Z-rcN` pre-release first:
   `prerelease: auto` and the `{{ if not .Prerelease }}` guards keep it off
   `:latest` and off GitHub's "Latest release".

## Security

Do not open a public issue for a vulnerability — see [SECURITY.md](SECURITY.md).
