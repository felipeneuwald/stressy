# stressy

[![CI](https://github.com/felipeneuwald/stressy/actions/workflows/ci.yml/badge.svg)](https://github.com/felipeneuwald/stressy/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/felipeneuwald/stressy)](https://github.com/felipeneuwald/stressy/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/felipeneuwald/stressy)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

stressy is a simple CPU stress test tool written in Go. It allows you to stress test your CPU cores by running intensive cryptographic computations.

## Why stressy

`stress-ng` is the more capable tool and this does not try to replace it: it
carries over 300 stressors, stressy has one. What stressy has instead is
deployability. It is a single static binary of about 2.8 MB with nothing to
install alongside it, published for twelve OS/architecture targets and as a
`FROM scratch` container image holding that binary and the licence and nothing
else — no distribution base to accumulate CVEs between releases, no package
manager, no shell, and a non-root UID baked in so a pod spec under the
`restricted` Pod Security Standard needs nothing beyond what the standard
already asks for.

Reach for `stress-ng` when you need to stress memory, I/O or a particular
instruction mix. Reach for stressy when what you want is CPU load inside a
container, on a node you would rather not install anything on.

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
# Load the machine: one worker per usable CPU, until interrupted
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
# Bounded run: 30 seconds, then the container exits on its own
docker run --rm ghcr.io/felipeneuwald/stressy:latest -t 30s

# Two CPUs' worth of load for five minutes
docker run --rm --cpus 2 ghcr.io/felipeneuwald/stressy:latest -t 5m

# Four workers regardless of what the container is allowed
docker run --rm ghcr.io/felipeneuwald/stressy:latest -w 4 -t 5m

# Using environment variables
docker run --rm -e STRESSY_WORKERS=4 -e STRESSY_TIMEOUT=5m ghcr.io/felipeneuwald/stressy:latest
```

Every one of those is bounded, and that is deliberate. Leave the timeout off and
stressy runs until it is interrupted — which at a terminal means Ctrl-C, but
from `docker run -d` means a container loading every CPU it can reach until
somebody thinks to run `docker stop`. If you want an unbounded run, start it
where you can see it.

A container with a CPU limit gets a worker count to match it: `docker run --cpus
2` on a 16-core host starts 2 workers, not 16, because the default is read from
the cgroup CPU quota rather than the host's core count. The floor is 2, so
`--cpus 1` also starts 2 — pass `-w 1` if you want exactly one. The same holds
for a Kubernetes `resources.limits.cpu`.

### Kubernetes

A `Job` is the shape this fits: it starts one pod, waits for it to exit 0 and
records the run as finished. That makes `-t` load-bearing rather than optional —
a container that never terminates leaves the Job running forever.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: stressy
spec:
  # A run that failed should not be retried indefinitely at full CPU.
  backoffLimit: 0
  # Clear the finished Job an hour after it completes.
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: stressy
          image: ghcr.io/felipeneuwald/stressy:latest
          args: ["-t", "60s"]
          resources:
            limits:
              cpu: "2"
              memory: 64Mi
            requests:
              cpu: "2"
              memory: 64Mi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            seccompProfile:
              type: RuntimeDefault
      securityContext:
        runAsNonRoot: true
```

```bash
kubectl apply -f stressy-job.yaml
kubectl logs -l batch.kubernetes.io/job-name=stressy
kubectl delete job stressy
```

There is no `-w` in that manifest on purpose. `limits.cpu: "2"` is a cgroup
quota, the worker count is read from the quota, and the pod starts two workers —
so the load follows the limit, and raising one raises the other. Setting `-w`
higher than the limit does not produce more load, only more throttling.

`requests` matching `limits` puts the pod in the Guaranteed QoS class. For a
workload whose entire purpose is to consume its quota that is the honest
declaration, and it keeps the pod off the top of the eviction list when it does
exactly what it was deployed to do. 64Mi is generous: a run measures under 5 MiB
resident, and the whole image is 2.8 MB.

The four `securityContext` fields are what the `restricted` Pod Security
Standard requires, and they are the whole of it — `runAsUser` is not among them,
because the image already runs as UID/GID `65532`. Worth knowing how this fails
if you drop them: the Job itself is admitted, with a warning that is easy to
miss, and then every pod its controller creates is rejected — so `kubectl get
job` shows `0/1` indefinitely with no pod to look at, and the reason is only in
the Job's events (`FailedCreate … is forbidden: violates PodSecurity
"restricted:latest"`, naming the fields it wants).

### The image

The image is `FROM scratch` — it holds the static binary and the licence, nothing
else. Two consequences worth knowing before you deploy it:

- It runs as UID/GID `65532`. The `restricted` Pod Security Standard requires
  `runAsNonRoot: true`, and the kubelet will not start a container under that
  setting if the image would run as root — so with a root image every pod spec
  has to pin `runAsUser` too. This one does not.
- There is no shell, so `docker exec` and `kubectl exec` into a running
  container will not work. Everything stressy reports, it reports on stdout at
  startup, and `--help` ships in the binary.

`:latest` only ever points at a full release; pre-release tags such as
`v0.4.0-rc1` publish under their own version tag and leave `:latest` alone.

### Available Flags

- `-w, --workers`: Number of parallel workers (must be 1 or greater). Defaults to the number of CPUs this process can use — the host's core count, narrowed by the CPU affinity mask, by a cgroup CPU limit if there is one, and by the `GOMAXPROCS` environment variable if it is set. `stressy --help` prints the number for the machine you run it on
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

## Contributing

Contributions are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) covers the build,
the checks CI runs before it will go green, and the conventions this repository
keeps — commit and branch naming, changelog entries, and the house rule that a
claim the documentation makes gets a test holding it to it.

## Security

Please do not open a public issue for a vulnerability.
[SECURITY.md](SECURITY.md) says what is in scope and how to report privately.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.