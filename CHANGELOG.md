# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The shutdown line names the signal: `Received SIGTERM, shutting down...`.
- `-r, --report` has to be `1s` or longer, and no longer than `--timeout`.
- The help output wraps at 80 columns instead of running past the margin.
- A rejected command line prints the flags without the examples block.
- `--workers` defaults to 1 again, not the number of CPUs available.
- Attached and grouped shorthands (`-w4`, `-t30s`, `-hv`) no longer parse.
- A rejected value echoes the spelling typed: `invalid value "abc" for flag -workers`.
- `--help` and the image description call stressy a lightweight CPU stress test tool.
- Release binaries ship as `tar.gz` archives, `zip` on Windows, licence inside.
- Building stressy needs Go 1.25 or newer, down from 1.26.

### Removed

- Environment-variable configuration: `STRESSY_WORKERS`, `STRESSY_TIMEOUT` and `STRESSY_REPORT`.
- A bare number as a duration: `-t 60` now has to be `-t 60s`.
- NetBSD and OpenBSD release binaries.
- cobra and pflag: one direct dependency left, and binaries 27% smaller.

### Fixed

- A signal that beats the expiring timeout exits 130 or 143, never 0.
- A rejected value is named once in the error, not twice.

## [0.5.0] - 2026-08-08

### Added

- `-r, --report` prints a progress line at an interval, e.g. `-r 30s`.
- An end-of-run summary: hashes computed, elapsed time and hashes per second.
- Examples in `stressy --help`, and a Kubernetes `Job` manifest in the README.
- `CONTRIBUTING.md` and `SECURITY.md`.

### Changed

- `--workers` now defaults to the number of CPUs available, not 1.
- Interrupted runs exit 130 (SIGINT) or 143 (SIGTERM) instead of 0.
- An indefinite run says how to stop it; a bounded run drops the `--help` hint.
- A bad environment variable names the variable: `invalid STRESSY_TIMEOUT: ...`.
- Release binaries are 43% smaller: 4.8 MB down to 2.8 MB.
- Docker examples in the README are bounded with `-t` and use `--rm`.

### Removed

- viper and `k8s.io/utils`; the module graph is 17 modules down to 4.

### Fixed

- Windows on ARM64 binaries are published again.
- `stressy --version` reports the real version instead of `0.0.0`.
- A signal arriving as the timeout expired could crash the process.
- Bad values say what a valid one looks like: `want a whole number, 1 or greater`.
- Non-flag arguments are rejected: `stressy 4` no longer runs the default silently.
- A runtime error prints one line instead of the whole help screen.
- `STRESSY_VERSION` and `STRESSY_HELP` no longer abort the run.
- The startup line reads `1 worker`, not `1 workers`.

### Security

- Every GitHub Action is pinned to a commit SHA.
- A `v*` tag publishes a release only when CI is green on that commit.

## [0.4.0] - 2026-08-03

### Changed

- `--timeout` takes a duration: `-t 5m`, `STRESSY_TIMEOUT=90s`. Bare seconds still work.
- The image is `FROM scratch` and runs as UID/GID 65532; no shell, so `docker exec` no longer works.
- `:latest` is published only for full releases, not for pre-releases such as `v0.4.0-rc1`.
- A pre-release tag no longer publishes as a full GitHub release.
- Release binaries are reproducible: `-trimpath` and a stamped commit timestamp.
- Release binaries are built with the latest stable Go.
- Containerised runs respect their CPU limit; Go 1.25 makes `GOMAXPROCS` cgroup-aware.
- Updated cobra, pflag, viper and golang.org/x/crypto to current versions.
- Release binaries are a third smaller: 7.2 MB down to 4.8 MB.
- Six indirect dependencies dropped, 18 down to 12.

### Removed

- The `go mod tidy` release hook, which stamped published binaries `v0.3.3+dirty`.
- The redundant `RUN chmod +x` image layer, replaced by `COPY --chmod=0755`.

### Fixed

- The MIT licence ships with the images and the release binaries.
- `go install github.com/felipeneuwald/stressy@latest` works; `package main` moved to the module root.
- `go build` in the repository root produces `stressy`, not `cmd`.
- A malformed environment variable such as `STRESSY_TIMEOUT=abc` is rejected instead of ignored.

### Security

- The image no longer carries an end-of-life Alpine base layer.

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

## [0.1.0] - 2020-04-01

### Added

- Initial release
- Basic CPU stress testing functionality
- Command-line interface for controlling stress parameters

[Unreleased]: https://github.com/felipeneuwald/stressy/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/felipeneuwald/stressy/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/felipeneuwald/stressy/compare/v0.3.3...v0.4.0
[0.3.3]: https://github.com/felipeneuwald/stressy/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/felipeneuwald/stressy/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/felipeneuwald/stressy/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/felipeneuwald/stressy/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/felipeneuwald/stressy/releases/tag/v0.2.0
