# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Upgraded to Go 1.23.3
- Updated golang.org/x/crypto dependency
- Removed binary files from git repository
- Added GoReleaser for automated releases
- Added GitHub Actions workflow for automated builds

## [2.0.1] - 2021-01-17

### Security
- Updated dependencies to fix vulnerabilities

### Changed
- Upgraded Go version

## [2.0.0] - 2021-01-12

### Added
- Docker support
- Makefile for easier build management
- Go modules support

### Changed
- Major refactoring of the codebase
- Updated documentation
- Improved build process

### Removed
- Vendor directory

## [1.0.0] - 2020

### Added
- Initial release
- CPU stress testing with simple loop & bcrypt
- Goroutine support
- Basic documentation

[Unreleased]: https://github.com/felipeneuwald/stressy/compare/v2.0.1...HEAD
[2.0.1]: https://github.com/felipeneuwald/stressy/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/felipeneuwald/stressy/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/felipeneuwald/stressy/releases/tag/v1.0.0
