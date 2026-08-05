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
- An end-of-run summary reporting hashes computed, elapsed time and rate
- An optional progress line on an interval, for runs long enough to check on
- Exit codes that tell an interrupted run from a completed one
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

# Print a progress line every 30 seconds
stressy -t 30m -r 30s
# or
stressy --timeout 30m --report 30s

# Using environment variables
export STRESSY_WORKERS=4
export STRESSY_TIMEOUT=5m
stressy
```

A flag given on the command line beats its environment variable, and an empty
variable counts as unset — so `STRESSY_WORKERS=${WORKERS}` with `WORKERS`
undefined leaves the default in place rather than failing the run.

### Output

A run says what it is about to do, why it stopped, and what it did:

```console
$ stressy -w 4 -t 60s
Starting CPU stress test with 4 workers for 60s
Timer expired, shutting down...
Computed 1324 hashes in 1m0.101s (22.0 hashes/s, 4 workers)
```

The summary is printed after every worker has finished the hash it was inside,
so it doubles as the confirmation that the shutdown the line above it announced
actually completed. Its elapsed time is measured rather than the `-t` you asked
for echoed back: a worker can only notice the deadline between hashes, so a run
ends up to one hash past it.

A run with no `-t` has no deadline to announce, so it says how to stop it
instead. The summary prints on that path too, so an interrupted run still
reports what it managed before it was cut short:

```console
$ stressy -w 4
Starting CPU stress test with 4 workers indefinitely
Press Ctrl+C or send SIGTERM to stop. Use --help for additional information
^C
Received signal, shutting down...
Computed 412 hashes in 18.734s (22.0 hashes/s, 4 workers)
```

Because bcrypt at a fixed cost is constant work per hash, that rate is a crude
but usable cross-node benchmark — run the same job on every node pool, and a
node hashing 30% slower is a finding.

Between those lines a run says nothing, however long it runs. On a half-hour
Kubernetes Job that means `kubectl logs` shows one line for the twenty-nine
minutes the pod is working, and the other two only once it has stopped. `-r,
--report` fills the gap with a progress line on an interval:

```console
$ stressy -w 4 -t 5m --report 1m
Starting CPU stress test with 4 workers for 5m0s
1m0.001s elapsed, 1320 hashes, 22.0 hashes/s
2m0.001s elapsed, 2640 hashes, 22.0 hashes/s
3m0.001s elapsed, 3960 hashes, 22.0 hashes/s
4m0.001s elapsed, 5280 hashes, 22.0 hashes/s
5m0.001s elapsed, 6596 hashes, 22.0 hashes/s
Timer expired, shutting down...
Computed 6600 hashes in 5m0.093s (22.0 hashes/s, 4 workers)
```

It is off unless you ask for it: a run given no `--report` prints what the first
two sessions above show and nothing more. Two things about the numbers, both of
which matter if you are watching the line rather than reading it afterwards. The
rate is cumulative — every hash since the run started over the whole elapsed
time, which is the same figure the summary ends with, so the last progress line
and the summary agree rather than differing by a window you cannot see. And the
elapsed time is measured at the moment the line is printed rather than rounded
to the interval, so a tick a starved process delivered late says so.

The last tick and the deadline fall due together and race, so the line above
`Timer expired` may or may not appear; its count is short of the summary's
either way, because the workers each finish the hash they are inside after the
deadline.

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

It also makes the [exit codes](#exit-codes) load-bearing. A pod that is evicted,
preempted, drained or `kubectl delete`d part-way through its run exits 143, not
0, so the Job records it as failed rather than `Complete` — which is what gives
the `backoffLimit: 0` below something to bound, and what lets `kubectl get job`
tell a pod that served its 60 seconds from one that was killed after 5.

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

That `kubectl logs` is the whole of what you can see from outside: the image is
`FROM scratch`, so there is no shell to `kubectl exec` in with. On a run long
enough to check on — a soak test, a load test driving an HPA — the pod's three
lines arrive at the very beginning and the very end, and nothing in between says
whether it is still working. `--report` is what gives the log something to show
while the run is happening:

```yaml
containers:
  - name: stressy
    image: ghcr.io/felipeneuwald/stressy:latest
    args: ["-t", "30m", "--report", "1m"]
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
  container will not work. Everything stressy reports, it reports on stdout —
  the configuration at startup, the summary at the end, and a progress line on
  an interval if you pass `--report` — and `--help` ships in the binary.
  `--report` exists because of this: with no shell to exec in with, stdout is
  the only in-band window into a container that is still running.

`:latest` only ever points at a full release; pre-release tags such as
`v0.4.0-rc1` publish under their own version tag and leave `:latest` alone.

### Available Flags

- `-w, --workers`: Number of parallel workers (must be 1 or greater). Defaults to the number of CPUs this process can use — the host's core count, narrowed by the CPU affinity mask, by a cgroup CPU limit if there is one, and by the `GOMAXPROCS` environment variable if it is set. `stressy --help` prints the number for the machine you run it on
- `-t, --timeout`: How long to run, as a duration such as `30s`, `5m` or `1h30m`. A bare number is read as seconds, so `-t 60` still means one minute. `0`, the default, runs until interrupted
- `-r, --report`: Print a progress line this often — elapsed time, hashes computed and rate. Takes the same duration spellings `--timeout` does, a bare number included. `0`, the default, prints none, which is what a run has always done
- `-h, --help`: Show help information
- `-v, --version`: Show version information

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The run served the whole `--timeout` it was given |
| `1` | The configuration was rejected — an unknown flag, an unparseable value, an unexpected argument — and no work was done |
| `130` | SIGINT cut the run short, which is 128 + 2 and what Ctrl-C sends |
| `143` | SIGTERM cut the run short, which is 128 + 15 and what `docker stop`, a `kubectl delete pod` and a node drain send |

`128 + signum` is the convention `timeout(1)`, the shells and every process
killed by a signal it does not handle already follow, so anything reading the
status needs to know nothing about stressy to read it. An unbounded run (`-t 0`)
is included: stopping one deliberately still reports 143, because from outside
the process the `docker stop` you meant and the eviction you did not are the same
event.

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