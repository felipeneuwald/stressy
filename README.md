# stressy

[![CI](https://github.com/felipeneuwald/stressy/actions/workflows/ci.yml/badge.svg)](https://github.com/felipeneuwald/stressy/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/felipeneuwald/stressy/badges/coverage.json)](https://github.com/felipeneuwald/stressy/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/felipeneuwald/stressy)](https://github.com/felipeneuwald/stressy/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/felipeneuwald/stressy)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A CPU stress tool. It loads as many CPUs as you ask it for with bcrypt hashing
and reports the rate, so the same run on two nodes is a comparison.

`stress-ng` has 300 stressors; stressy has one and deploys anywhere — a 2.1 MB
static binary for eight OS/architecture targets, and a `FROM scratch` image
with no base layer, no package manager, no shell and a non-root UID.

## Install

```bash
go install github.com/felipeneuwald/stressy@latest
docker pull ghcr.io/felipeneuwald/stressy:latest
```

Or a prebuilt archive from the
[releases](https://github.com/felipeneuwald/stressy/releases) — `tar.gz` per
unix target, `zip` for windows, each holding the binary, the licence and this
README:

```bash
# It extracts flat, so naming the binary keeps its LICENSE and README out of the way
tar -xzf stressy_Linux_x86_64.tar.gz stressy
./stressy -t 5s
```

## Usage

```bash
# One worker until interrupted
stressy

# Four workers for five minutes
stressy -w 4 -t 5m

# Any Go duration
stressy -t 1h30m

# A progress line every 30 seconds
stressy -t 30m -r 30s
```

Every setting is a flag, and none of them is inferred. `-w` defaults to `1`, so
a bare `stressy` loads one CPU on a laptop, in a container and in a pod alike;
to load the whole machine, say so with `stressy -w $(nproc)`. stressy reads no
environment variable, no config file and no positional argument, so a command
line is the whole of what a run was given.

### Output

A run says what it is about to do, why it stopped, and what it did:

```console
$ stressy -w 4 -t 60s
Starting CPU stress test with 4 workers for 60s
Timer expired, shutting down...
Computed 1324 hashes in 1m0.101s (22.0 hashes/s, 4 workers)
```

A run with no `-t` says how to stop it, and still reports what it managed:

```console
$ stressy -w 4
Starting CPU stress test with 4 workers indefinitely
Press Ctrl+C or send SIGTERM to stop. Use --help for additional information
^C
Received SIGINT, shutting down...
Computed 412 hashes in 18.734s (22.0 hashes/s, 4 workers)
```

Because bcrypt at a fixed cost is constant work per hash, that rate is a crude
cross-node benchmark: a node hashing 30% slower is a finding.

That cost is 12, and it is fixed for the whole of 1.x. It is the unit every
number above is quoted in, so a figure recorded today and a figure recorded a
year from now are the same measurement; moving it would halve or double every
published number, which makes it a major version bump and nothing less. It is
not `bcrypt.MaxCost`, which is about 26 hours a hash: no harder on a core, and
long enough that a worker would never notice the run had ended.

Between those lines a run says nothing, so `-r, --report` fills the gap:

```console
$ stressy -w 4 -t 2m --report 1m
Starting CPU stress test with 4 workers for 2m0s
1m0.001s elapsed, 1320 hashes, 22.0 hashes/s
2m0.001s elapsed, 2636 hashes, 22.0 hashes/s
Timer expired, shutting down...
Computed 2640 hashes in 2m0.093s (22.0 hashes/s, 4 workers)
```

The interval has to be `1s` or longer, and no longer than `--timeout` where
there is one: below a second a run spends itself formatting rather than hashing,
and past the timeout the ticker never fires, which is what `-r 1s` mistyped as
`-r 1m` looks like — three lines, exit 0 and nothing to correct it by. Both are
rejected before any worker starts. A run with no `-t` outlives every interval,
so it takes any: `stressy -r 5m` reports until you stop it.

### The output is the interface

There is no `--json`. The lines above are what a script reads, and their wording
is stable for 1.x: rewording one is a breaking change and takes a major version
bump. Two of them carry a rate, so a script after the figure for the whole run
matches the summary — the line that starts with `Computed ` — rather than
`hashes/s`, which every progress line carries too.

### Containers

```bash
# Bounded run: 30 seconds, then the container exits on its own
docker run --rm ghcr.io/felipeneuwald/stressy:latest -t 30s

# Two CPUs' worth of load for five minutes
docker run --rm --cpus 2 ghcr.io/felipeneuwald/stressy:latest -w 2 -t 5m
```

Both are bounded, deliberately: with no timeout, `docker run -d` leaves a
container hashing until somebody runs `docker stop`.

The worker count is `-w` and nothing else — no CPU quota is read, so `--cpus 2`
without a `-w 2` loads one CPU and pays for two. Match `-w` to the limit: above
it the extra workers buy throttling rather than load, below it the limit is
partly idle.

In Kubernetes a `Job` is the shape this fits: one pod, run to exit 0, recorded as
finished — which makes `-t` and the [exit codes](#exit-codes) load-bearing, since
a pod evicted, preempted or `kubectl delete`d part-way through exits 143.

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
          # -w matches the cpu limit below; nothing derives one from the other.
          args: ["-w", "2", "-t", "60s"]
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
```

Those four `securityContext` fields are what the `restricted` Pod Security
Standard requires, and the whole of it: `runAsUser` is not among them, because
the image already runs as UID/GID `65532`. It is `FROM scratch`, so there is no
shell to `kubectl exec` in with and stdout is the only view you have of a run —
which is what `--report` is for, and why the shutdown line names the signal: an
evicted pod logs `Received SIGTERM`, which is the `143` it goes on to exit with.
`:latest` only ever points at a full release.

### Available Flags

- `-w, --workers`: Number of parallel workers (must be 1 or greater). `1`, the default, on every machine: nothing is read from the core count, the CPU affinity mask or a cgroup limit, so the number a run uses is the number you typed
- `-t, --timeout`: How long to run, as a duration such as `30s`, `5m` or `1h30m`. `0`, the default, runs until interrupted
- `-r, --report`: Print a progress line this often — elapsed time, hashes computed and rate. Takes the same duration spellings `--timeout` does, no shorter than `1s` and, on a bounded run, no longer than `--timeout`. `0`, the default, prints none, which is what a run has always done
- `-h, --help`: Show help information
- `-v, --version`: Show version information

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The run served the whole `--timeout` it was given |
| `1` | The configuration was rejected — an unknown flag, an unparseable or out-of-range value, an unexpected argument — and no work was done |
| `130` | SIGINT cut the run short, which is 128 + 2 and what Ctrl-C sends |
| `143` | SIGTERM cut the run short, which is 128 + 15 and what `docker stop`, a `kubectl delete pod` and a node drain send |

## Building from Source

```bash
git clone https://github.com/felipeneuwald/stressy.git
cd stressy
go build
./stressy -t 5s
```

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) has the checks CI runs and the conventions.

## Security

Please do not open a public issue for a vulnerability. [SECURITY.md](SECURITY.md)
says what is in scope and how to report privately.

## License

MIT — see [LICENSE](LICENSE).
