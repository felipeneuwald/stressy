# stressy

stressy is a simple CPU stress test tool written in Go. It allows you to stress test your CPU cores by running intensive computations.

## Features

- Simple and lightweight CPU stress testing
- Configurable number of CPU cores to stress
- Available as both binary and Docker container
- Cross-platform support (Linux, macOS, Windows)
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

### Binary

```bash
# Stress test all CPU cores
stressy

# Stress test specific number of cores
stressy -c 2  # Stress 2 CPU cores
```

### Docker

```bash
# Stress test all CPU cores
docker run ghcr.io/felipeneuwald/stressy:latest

# Stress test specific number of cores
docker run ghcr.io/felipeneuwald/stressy:latest -c 2
```

## Building from Source

```bash
# Clone the repository
git clone https://github.com/felipeneuwald/stressy.git
cd stressy

# Build
make build

# Run tests
make test
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.