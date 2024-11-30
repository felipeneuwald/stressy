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

# Run for 60 seconds
stressy -t 60
# or
stressy --timeout 60

# Combine workers and timeout
stressy -w 4 -t 60

# Using environment variables
export STRESSY_WORKERS=4
export STRESSY_TIMEOUT=60
stressy
```

### Docker

```bash
# Start stress test with default settings
docker run ghcr.io/felipeneuwald/stressy:latest

# Use 4 parallel workers for 60 seconds
docker run ghcr.io/felipeneuwald/stressy:latest -w 4 -t 60

# Using environment variables
docker run -e STRESSY_WORKERS=4 -e STRESSY_TIMEOUT=60 ghcr.io/felipeneuwald/stressy:latest
```

### Available Flags

- `-w, --workers`: Number of parallel workers (must be 1 or greater)
- `-t, --timeout`: Test duration in seconds (0 for indefinite, must be 0 or greater)
- `-h, --help`: Show help information
- `-v, --version`: Show version information

## Building from Source

```bash
# Clone the repository
git clone https://github.com/felipeneuwald/stressy.git
cd stressy

# Build and run
go build ./cmd
./stressy
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.