# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- CI workflow running lint, formatting, `go vet`, module tidiness and verification, build, `go test -race` and govulncheck on every push to main and every pull request
- `.github/dependabot.yml` checking Go modules and GitHub Actions `uses:` refs weekly, so dependency security fixes arrive as pull requests instead of waiting for someone to notice an advisory. Each of those pull requests also runs CI, which is what keeps govulncheck scanning during quiet periods now that the nightly schedule is gone
- `.golangci.yml` configuring golangci-lint as the single Go gate for both formatting and linting
- Unit tests for the flag package (registration, environment binding, allowed-value validation, duration parsing in both spellings), for stress test configuration validation, and CLI-level tests covering flag/environment variable precedence

### Changed
- `--timeout` now takes a duration, so `-t 5m` and `STRESSY_TIMEOUT=90s` say what they look like they say. Bare seconds still work — `-t 60` is a minute, exactly as before — because `time.ParseDuration` rejects a unit-less number, so the two spellings cannot be confused for one another and no existing command line or environment variable had to change. This also closes the sharpest edge of the malformed-value bug below: `STRESSY_TIMEOUT=60s` was the natural thing to type and the one thing that silently produced an endless run (#26)
- The Docker image is now `FROM scratch` rather than Alpine, and runs as UID/GID `65532` instead of root. The `restricted` Pod Security Standard requires `runAsNonRoot: true`, and the kubelet will not start a container under that setting if the image would run as root — so every pod spec running the old image had to pin `runAsUser` as well. For a tool whose usual deployment target is a cluster that was a scheduling friction rather than an exploitation risk, but it was the sharper of the two. Going to `scratch` also retires the base image as a source of CVEs entirely: images are only rebuilt on a release tag, so any distribution base accumulated advisories between releases with nothing to rebuild it. The binary is built `CGO_ENABLED=0` and needs no userland. The trade-off is that there is no shell, so `docker exec` and `kubectl exec` into a running container no longer work (#21)
- `ghcr.io/felipeneuwald/stressy:latest` is now only published for full releases. Tagging a pre-release such as `v0.4.0-rc1` used to repoint `:latest` — the tag `docker run` resolves by default — at a release candidate, and publish it as a full GitHub release; both are now guarded (#23)
- Release binaries are built with `-trimpath` and stamped with the commit timestamp, so the same commit built twice produces the same artefact instead of embedding the builder's absolute paths and the wall-clock time of the release job (#23)
- Release and vulnerability scan workflows now build with the latest stable Go instead of pinning 1.23.6, a pin that never took effect
- Raised the go.mod Go directive to 1.26, which enables the cgroup-aware `GOMAXPROCS` default so containerised runs respect their CPU limit instead of sizing to the host
- Updated cobra 1.8.1 to 1.10.2, pflag 1.0.5 to 1.0.10, viper 1.19.0 to 1.21.0 and golang.org/x/crypto 0.46.0 to 0.54.0 — the first updates to arrive on their own rather than after someone noticed an advisory. Nothing a user touches changed: help text, version output, unknown-flag errors and invalid-value errors are byte-identical across the bump. Nor is this a security update in disguise; every x/crypto advisory fixed in that range is in `ssh`, and the only package this project imports is `bcrypt`, whose source is identical in both versions
- Release binaries are roughly a third smaller, 7.2 MB down to 4.8 MB when built with the release `-s -w` flags, because cobra 1.9 lets the linker drop `text/template` for commands that use the default help templates
- Six indirect dependencies dropped, 18 down to 12: viper 1.21 no longer pulls in the HCL, Java-properties and INI config decoders, `slog-shim`, `multierr` or `golang.org/x/exp`, and it moved `mapstructure` and YAML onto maintained forks. None of those decoders were ever reachable here — viper reads `STRESSY_WORKERS` and `STRESSY_TIMEOUT` from the environment and never opens a config file

### Removed
- The nightly vulnerability scan workflow. GitHub disables schedule-triggered workflows in public repositories after 60 days of inactivity, which had already happened, so it had been reporting nothing for months while appearing healthy. govulncheck now runs in CI on every push and pull request instead
- The `go mod tidy` release hook in `.goreleaser.yaml`. It ran after goreleaser's dirty-tree check, so a drift it corrected could not fail the release — it rewrote `go.mod` mid-build, shipped a dependency graph nobody had reviewed and stamped the binary `v0.3.3+dirty`, which is visible in `go version -m` on the published artefact. Tidiness is checked in CI instead, where a drift is a red pull request (#23)
- The `docker` entry in `.github/dependabot.yml`, which has no base image to watch now that the Dockerfile is `FROM scratch`. The commented-out block records what to restore if a base image ever returns (#21)
- The redundant `RUN chmod +x` layer in the Dockerfile, replaced by `COPY --chmod=0755`. Besides being a layer for nothing, a `RUN` forced QEMU emulation during the arm64 cross-build (#21)

### Fixed
- The MIT licence text now ships with the artefacts it covers. `.goreleaser.yaml` staged `LICENSE` into the Docker build context, but no `COPY` picked it up and `formats: binary` means there is no archive to carry it either — so it reached neither the images nor the binaries. The Dockerfile now copies it to `/LICENSE`, and it is uploaded as a release asset alongside the binaries (#23)
- `go install github.com/felipeneuwald/stressy@latest`, the README's primary installation instruction, failed with "module found (v0.3.3), but does not contain package": `package main` lived in `cmd/`, so the module root held no installable package and the path the README advertised could not resolve. The main package now sits at the module root, so that command works as written; `main: ./cmd` in `.goreleaser.yaml` moved with it. Release artefacts and images are unaffected — same binary, same name, same import path for every `internal/` package (#9)
- The README's build instruction, `go build ./cmd` followed by `./stressy`, produced a binary named `cmd` and then ran something that was not there. `go build` in the repo root now produces `stressy` (#9)
- A malformed environment variable (for example `STRESSY_TIMEOUT=abc`) was silently discarded, leaving the flag at zero and turning a bounded run into an indefinite one that exited 0; it is now rejected with a message naming the flag and the value
- gofmt formatting in `internal/flag/load.go` and `internal/stressy/stressy.go`

### Security
- Retired the Docker base image. Published `ghcr.io/felipeneuwald/stressy` images had been built on Alpine 3.19, which left support on 2025-11-01, so they carried a base layer that no longer received security fixes — and scanners flag an end-of-life release on its own regardless of which CVEs sit in it. That was first fixed by moving to Alpine 3.24; the image is now `FROM scratch` instead, which removes the class of problem rather than resetting its clock (see Changed). The binary is static and was never affected — this was always about the rest of the image, which no longer exists

## [0.3.3] - 2026-01-06
### Security
- Updated golang.org/x/crypto from v0.31.0 to v0.46.0 to address vulnerabilities:
  - CVE-2025-22869: DoS via slow or incomplete SSH key exchange (high severity)
  - CVE-2025-58181: Unbounded memory consumption in SSH (medium severity)
  - CVE-2025-47914: Panic on malformed SSH agent message (medium severity)

## [0.3.2] - 2025-02-09
### Security
- Updated Go version from 1.23.3 to 1.23.6 to address vulnerabilities GO-2025-3447 (timing sidechannel in P-256) and GO-2025-3373 (IPv6 zone ID URI constraints bypass)

## [0.3.1] - 2024-12-12
### Security
- Updated golang.org/x/crypto from v0.29.0 to v0.31.0 to address vulnerability GO-2024-3321

## [0.3.0] - 2024-11-30
### Added
- Comprehensive code documentation
- Validation for configuration parameters
- Informative startup messages showing test configuration

### Changed
- Improved error handling and validation messages
- Simplified build process by removing Makefile
- Enhanced CLI help messages and documentation
- Allow indefinite stress testing with timeout=0
- Refactored flag package for better type safety and validation

### Fixed
- Workers validation to require 1 or more workers
- Timeout validation to allow 0 (indefinite) or greater values

## [0.2.0] - 2024-11-24
### Added
- Multi-platform Docker image support via GoReleaser
- Automated GitHub Actions workflow for releases
- Docker images for AMD64 and ARM64 architectures

### Changed
- Updated project to use Go 1.23
- Updated Dockerfile to use Alpine 3.19
- Improved build and release process with GoReleaser
- Updated golang.org/x/crypto dependency to latest version

## [0.1.0] - 2020-03-22
### Added
- Initial release
- Basic CPU stress testing functionality
- Command-line interface for controlling stress parameters

[Unreleased]: https://github.com/felipeneuwald/stressy/compare/v0.3.3...HEAD
[0.3.3]: https://github.com/felipeneuwald/stressy/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/felipeneuwald/stressy/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/felipeneuwald/stressy/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/felipeneuwald/stressy/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/felipeneuwald/stressy/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/felipeneuwald/stressy/releases/tag/v0.1.0
