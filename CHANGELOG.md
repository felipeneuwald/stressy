# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- CI workflow running lint, formatting, `go vet`, module tidiness and verification, build, `go test -race` and govulncheck on every push to main and every pull request
- `.github/dependabot.yml` checking Go modules, GitHub Actions `uses:` refs and the Dockerfile base image weekly, so dependency and base image security fixes arrive as pull requests instead of waiting for someone to notice an advisory. Each of those pull requests also runs CI, which is what keeps govulncheck scanning during quiet periods now that the nightly schedule is gone
- `.golangci.yml` configuring golangci-lint as the single Go gate for both formatting and linting
- Unit tests for the flag package (registration, environment binding, allowed-value validation), for stress test configuration validation, and CLI-level tests covering flag/environment variable precedence

### Changed
- Release and vulnerability scan workflows now build with the latest stable Go instead of pinning 1.23.6, a pin that never took effect
- Raised the go.mod Go directive to 1.26, which enables the cgroup-aware `GOMAXPROCS` default so containerised runs respect their CPU limit instead of sizing to the host
- Updated cobra 1.8.1 to 1.10.2, pflag 1.0.5 to 1.0.10, viper 1.19.0 to 1.21.0 and golang.org/x/crypto 0.46.0 to 0.54.0 — the first updates to arrive on their own rather than after someone noticed an advisory. Nothing a user touches changed: help text, version output, unknown-flag errors and invalid-value errors are byte-identical across the bump. Nor is this a security update in disguise; every x/crypto advisory fixed in that range is in `ssh`, and the only package this project imports is `bcrypt`, whose source is identical in both versions
- Release binaries are roughly a third smaller, 7.2 MB down to 4.8 MB when built with the release `-s -w` flags, because cobra 1.9 lets the linker drop `text/template` for commands that use the default help templates
- Six indirect dependencies dropped, 18 down to 12: viper 1.21 no longer pulls in the HCL, Java-properties and INI config decoders, `slog-shim`, `multierr` or `golang.org/x/exp`, and it moved `mapstructure` and YAML onto maintained forks. None of those decoders were ever reachable here — viper reads `STRESSY_WORKERS` and `STRESSY_TIMEOUT` from the environment and never opens a config file

### Removed
- The nightly vulnerability scan workflow. GitHub disables schedule-triggered workflows in public repositories after 60 days of inactivity, which had already happened, so it had been reporting nothing for months while appearing healthy. govulncheck now runs in CI on every push and pull request instead

### Fixed
- `go install github.com/felipeneuwald/stressy@latest`, the README's primary installation instruction, failed with "module found (v0.3.3), but does not contain package": `package main` lived in `cmd/`, so the module root held no installable package and the path the README advertised could not resolve. The main package now sits at the module root, so that command works as written; `main: ./cmd` in `.goreleaser.yaml` moved with it. Release artefacts and images are unaffected — same binary, same name, same import path for every `internal/` package (#9)
- The README's build instruction, `go build ./cmd` followed by `./stressy`, produced a binary named `cmd` and then ran something that was not there. `go build` in the repo root now produces `stressy` (#9)
- A malformed environment variable (for example `STRESSY_TIMEOUT=abc`) was silently discarded, leaving the flag at zero and turning a bounded run into an indefinite one that exited 0; it is now rejected with a message naming the flag and the value
- gofmt formatting in `internal/flag/load.go` and `internal/stressy/stressy.go`

### Security
- Updated the Docker base image from Alpine 3.19 to 3.24. Alpine 3.19 left support on 2025-11-01, so published `ghcr.io/felipeneuwald/stressy` images had been carrying a base layer that no longer received security fixes, and scanners flag an end-of-life release on its own regardless of which CVEs sit in it. The binary is static and was never affected — this is entirely about the rest of the image

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
