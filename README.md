# stressy

stressy is a simple CPU stress test tool written in Go. It allows you to stress test your CPU cores by running intensive cryptographic computations.

## Features

- Simple and lightweight CPU stress testing
- Configurable number of parallel workers
- Configurable test duration with support for indefinite testing
- Environment variable configuration support
- Available as both binary and Docker container
- Cross-platform support (Linux, macOS, Windows, FreeBSD, NetBSD, OpenBSD)
- Multi-architecture support (AMD64, ARM64)

## Installation

### Using Go

```bash
go install github.com/felipeneuwald/stressy@latest
```

### Using Docker

```bash
# AMD64
docker pull ghcr.io/felipeneuwald/stressy:latest-amd64

# ARM64
docker pull ghcr.io/felipeneuwald/stressy:latest-arm64

# Multi-arch (automatically selects the right architecture)
docker pull ghcr.io/felipeneuwald/stressy:latest
```

### Binary Releases

Download the latest binary for your platform from the [releases page](https://github.com/felipeneuwald/stressy/releases).

## Usage

```bash
# Start stress test with default settings (1 worker)
stressy

# Use 4 parallel workers
stressy -w 4
# or
stressy --workers 4

# Run for five minutes
stressy -t 5m
# or
stressy --timeout 5m

# Any Go duration works: 90s, 1h30m, 250ms
stressy -t 1h30m

# A bare number is read as seconds, so pre-0.4 command lines keep working
stressy -t 60

# Combine workers and timeout
stressy -w 4 -t 5m

# Using environment variables
export STRESSY_WORKERS=4
export STRESSY_TIMEOUT=5m
stressy
```

### Docker

```bash
# Start stress test with default settings
docker run ghcr.io/felipeneuwald/stressy:latest

# Use 4 parallel workers for five minutes
docker run ghcr.io/felipeneuwald/stressy:latest -w 4 -t 5m

# Using environment variables
docker run -e STRESSY_WORKERS=4 -e STRESSY_TIMEOUT=5m ghcr.io/felipeneuwald/stressy:latest
```

The image is `FROM scratch` — it holds the static binary and the licence, nothing
else. Two consequences worth knowing before you deploy it:

- It runs as UID/GID `65532`, so it is admissible into a Kubernetes namespace
  enforcing the `restricted` Pod Security Standard without a `securityContext`
  override.
- There is no shell, so `docker exec` and `kubectl exec` into a running
  container will not work. Everything stressy reports, it reports on stdout at
  startup.

`:latest` only ever points at a full release; pre-release tags such as
`v0.4.0-rc1` publish under their own version tag and leave `:latest` alone.

### Available Flags

- `-w, --workers`: Number of parallel workers (must be 1 or greater)
- `-t, --timeout`: How long to run, as a duration such as `30s`, `5m` or `1h30m`. A bare number is read as seconds, so `-t 60` still means one minute. `0`, the default, runs until interrupted
- `-h, --help`: Show help information
- `-v, --version`: Show version information

## Building from Source

```bash
# Clone the repository
git clone https://github.com/felipeneuwald/stressy.git
cd stressy

# Build and run
go build
./stressy
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.