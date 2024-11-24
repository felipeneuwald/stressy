# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/felipeneuwald/stressy/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/felipeneuwald/stressy/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/felipeneuwald/stressy/releases/tag/v0.1.0
