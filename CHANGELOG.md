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

### Removed
- The nightly vulnerability scan workflow. GitHub disables schedule-triggered workflows in public repositories after 60 days of inactivity, which had already happened, so it had been reporting nothing for months while appearing healthy. govulncheck now runs in CI on every push and pull request instead

### Fixed
- A malformed environment variable (for example `STRESSY_TIMEOUT=abc`) was silently discarded, leaving the flag at zero and turning a bounded run into an indefinite one that exited 0; it is now rejected with a message naming the flag and the value
- gofmt formatting in `internal/flag/load.go` and `internal/stressy/stressy.go`

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
