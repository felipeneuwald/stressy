# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Tests covering `Run`'s shutdown, which nothing exercised before: both signal paths, the timeout path, a signal racing the timeout, cancellation observed mid-hash and on an already-cancelled context, and a second `Run` on the same instance. Statement coverage of `internal/stressy` goes from 24.2% to 97.3%, and the remainder is one unreachable `panic`. These are the first tests to run Run's goroutines, which is what the `-race` flag in CI was put there for (#14, #15)

### Changed
- `--workers` defaults to the number of CPUs this process can use, where it defaulted to 1. A CPU stress tool whose bare invocation loads one core is answering a question nobody asks: on the 18-core machine this was written on, `stressy` produced about 6% of the load the tool exists to produce, and the README's own first example presented that as the point rather than as a surprise. The default is `runtime.GOMAXPROCS(0)`, not the `runtime.NumCPU()` the issue proposed, and the difference between them is the containerised case rather than a detail — NumCPU reports the machine's logical CPUs and knows nothing about a cgroup CPU quota, so on a 64-core node under `limits.cpu: 2` it would have started 64 workers to fight over two cores' worth of quota, which is heavier throttling and burstier, less representative load than the single worker it replaced. Since the go.mod language version reached 1.25 in 0.4.0, the Go runtime's own default already accounts for the cgroup quota, the CPU affinity mask and the `GOMAXPROCS` environment variable, and what it yields is precisely the number of workers that can be running at once. Measured under Docker on an 8-CPU VM: unlimited starts 8 workers, `--cpus 2` starts 2 and `--cpus 1` starts 2 as well, because the runtime never defaults below 2 unless the machine itself has fewer — `-w 1` is still exactly one worker. This is the user-visible behaviour change in this release, and the reason the next tag is a minor version rather than a patch (#24, #25)
- Workers hash at bcrypt cost 12 rather than `bcrypt.MaxCost`. bcrypt doubles its work per cost increment, so cost 31 is a single `GenerateFromPassword` call that runs for about 26 hours — measured on an M-series core at 177 ms for cost 12 and 2.855 s for cost 16, with the doubling holding across the range, which puts cost 31 at 2.855 s × 2¹⁵. The loop body therefore ran exactly once per worker and the cancellation check at the top of it was reached, in practice, never. This is not a reduction in load: bcrypt is the same key-expansion loop at every cost, run more or fewer times, and the measurements agree — four workers for two seconds consume 3.99 times wall-clock in CPU where they used to consume 4.00, and eight workers 7.97 against 7.98. What it buys is that a worker notices a cancelled context within one hash instead of within a day (#15)
- `Run` waits for its workers before returning, so the graceful shutdown the package documents is the one it performs, rather than the process exiting out from under goroutines still mid-hash. The cost is that a run now ends up to one hash after its deadline — measured at 0.11 s past a 2 s timeout with one worker and 0.05 s with four, against a worst case of one hash. At `bcrypt.MaxCost` this was not an option: waiting for a worker would have taken 26 hours, which is the sense in which #15 blocked #14 (#14, #15)
- The hashed input is a constant computed once per worker instead of `fmt.Sprintf("%v", time.Now())` per iteration. bcrypt generates a fresh random salt on every call, so a varying input never contributed anything, and formatting a timestamp is not the load this tool exists to generate. It also decouples the input from bcrypt's 72-byte limit, which a formatted `time.Now()` stayed under by roughly ten characters rather than by design — and hoisting it would have turned that from a panic no one would live to see into one at worker startup (#15)
- Release binaries are 43% smaller, 4,849,266 bytes down to 2,758,210 when built with the release `-s -w` flags, and both container images shrink by the same 2 MB. That is the whole of what viper was costing: dropping it removes no functionality, because none of what it linked in was reachable (see Removed) (#18)
- A rejected environment variable now names the variable instead of the flag — `invalid STRESSY_TIMEOUT: …` where it used to say `invalid configured value for --timeout: …`. The environment is the only thing that code path reads, so the variable is what the operator can actually go and correct; pflag's own wrapper still names the flag inside the same message, so nothing was lost. Everything else a user sees is byte-identical to 0.4.0: `--help`, `--version`, unknown-flag errors, invalid-flag-value errors and every configured run (#18)
- Flags are registered on the cobra command directly, in the two lines cobra asks for, rather than through a generic definition layer. `--workers` and `--timeout` are the only flags this tool has ever had, and the layer that registered them cost more than it saved (see Removed). Nothing an operator touches changed: `--help`, `--version`, unknown-flag errors, invalid-flag-value errors, rejected environment variables and every configured run are byte-for-byte what 0.4.0 printed. The binary is 368 bytes smaller, which is the honest size of it — this is a readability and dependency change, not a size one (#20)

### Removed
- viper, and with it twelve of the seventeen modules in `go.mod` — eleven of the twelve `// indirect` entries were reachable through nothing else. What is left is cobra, pflag, `golang.org/x/crypto`, `k8s.io/utils` and cobra's own `mousetrap`. viper was doing exactly one thing here: `AutomaticEnv` over `STRESSY_WORKERS` and `STRESSY_TIMEOUT`. Nothing in this project reads a config file, watches the filesystem or parses TOML, YAML or dotenv, so every decoder those two variables dragged in was linked in dead. `internal/flag.Bind` now takes an environment prefix and reads `os.LookupEnv` itself. The three behaviours that were viper's rather than this project's are kept deliberately and covered by tests: command-line flags still beat the environment, a dash in a flag name still maps to an underscore in the variable, and an empty value is still treated as unset — that last one because `STRESSY_WORKERS=${WORKERS}` with `WORKERS` undefined is a common shape in compose files and pod specs, and rejecting it would break deployments that run today (#18)
- `golang.org/x/text` and `golang.org/x/sys` from the module graph entirely. Neither was imported here; they entered through viper's `afero` and `fsnotify`, and they are what container scanners flag on the published images (#18)
- The `internal/flag` package — 230 lines and five files wrapping cobra and pflag so that two flags could be declared as struct literals in a `[]interface{}` and registered by a type switch. Most of it was never reached: `flag.String` had no caller anywhere in the repository, and the `AllowedValues` field on both types was never set by anything, which left the usage-text augmentation, the two slice-formatting helpers that fed it and the whole of `Validate` as machinery for a constraint no flag in this tool has. `Validate` walked every registered flag against every definition on every run to decide there was nothing to check. The two parts that did work moved into `package main` unchanged in behaviour and now sit next to their only caller: environment resolution in `env.go` (`bindEnv`, `envName`) and the duration type that accepts both `5m` and bare seconds in `duration.go` (`durationValue`, `parseDuration`). Their tests moved with them; the ones that only covered the deleted surface went with it. If this pattern is wanted in another project it belongs in its own module, where it can be depended on rather than copied (#20)
- `k8s.io/utils` from `go.mod`. It was a direct dependency on a `k8s.io` module for one `ptr.To` call, in `validate.go`'s unsupported-type branch — a branch that could not be reached, since every type the switch was ever handed had a case (#19). `go.mod` is now three direct dependencies and one indirect: cobra, pflag, `golang.org/x/crypto`, and cobra's own `mousetrap`
- The unsupported-flag-type detection in `validate.go`, which went with the file. It wrote the offending type name into a variable captured by a `VisitAll` callback that runs once per registered flag, so whether a bad definition was reported at all depended on the order the flags happened to be visited in. It never fired, for the reason above (#17d)

### Fixed
- Release binaries are published for Windows on ARM64. The README advertises six operating systems on one bullet and two architectures on the next, which reads as a twelve-target matrix, and eleven of those targets were built: `.goreleaser.yaml` carried an `ignore:` block excluding `windows/arm64`, present since the first release config in November 2024 and explained nowhere in the repository since. A Windows-on-ARM user therefore found no `stressy_Windows_arm64` on the releases page, or took the amd64 binary and ran it under Windows' x86 emulation — where the measurements a CPU stress tool exists to produce mean nothing, since the load being measured is the emulator's as much as the machine's. Nothing had to be worked around to fix it: the target cross-compiles at `CGO_ENABLED=0` like the other eleven, verified across the whole matrix, and costs one more 2.8 MB artefact per release. A missing artefact is silent — the release is green and the download list is simply one row shorter — so a test now reads the `goos` and `goarch` lists out of `.goreleaser.yaml` and the two support bullets out of `README.md` and fails if they stop describing the same matrix, or if an `ignore:` block reappears with no exception stated in the README (#28)
- `stressy --version` reports the version of binaries that goreleaser did not build, where it printed `0.0.0`. That was every `go install github.com/felipeneuwald/stressy@latest` — the README's first installation instruction, and one that only started resolving at all in 0.4.0 (#9) — and every local `go build`. Nothing had to be added to the build to fix it: `debug.ReadBuildInfo().Main.Version` is populated on every build path and was simply never read. The version is now taken from the ldflags value when there is one and from build info otherwise, with the leading `v` stripped so that a release spells itself the same way whichever path produced the binary: a goreleaser artefact still reports `0.4.0`, `go install ...@v0.4.0` now reports `0.4.0` too, a build from a modified tree reports `0.4.0+dirty`, and one from a commit past the last tag reports the pseudo-version identifying that commit. The last two are deliberate — a binary that is not the release it is nearest to should not claim to be it. `go build -buildvcs=false`, and any binary carrying no build information at all, reports `devel`, which says what it is where `0.0.0` looked like a release version that had been cut. The ldflags target is renamed from `main.version` to `main.injected` and `.goreleaser.yaml` sets the new name; because a `-X` naming a variable that does not exist is dropped by the linker rather than failing the link, the two files disagreeing would have been visible only in a published release, so a test now checks the release config against the source (#40)
- A runtime failure no longer prints the entire help screen after its error message: `stressy -w 0` answered a one-line configuration mistake with the full flag list, because `SilenceUsage` was never set. It is set at the top of `PreRunE` rather than on the command literal, and that is the whole of the care this needed — cobra has finished parsing flags and validating arguments by the time `PreRunE` runs, so everything from there on is a runtime error by construction, while the two paths where usage genuinely is the answer keep printing it. Setting the field on the literal, which is the obvious spelling, would also have silenced an unknown flag and an unparseable flag value, leaving a user who mistyped a flag name without the list that names the right one. A rejected environment variable lands on the silent side, since `bindEnv` runs inside `PreRunE`: it is a configuration error, and its message already names the variable and the value (#17a)
- Arguments that are not flags are rejected rather than silently discarded. `stressy foo bar 4` started a default run and said nothing about any of the three, and `stressy 4` — a reasonable guess at how the worker count is spelled — ran the default number of workers instead. The message names what was rejected, `unexpected argument "4": stressy takes flags only`, where `cobra.NoArgs` reports `unknown command "4"` and sends the user of a command that has no subcommands looking for one that cannot exist. The usage screen still follows, so the flag they wanted is on the same screen as the error (#17b)
- The startup line reads `1 worker` rather than `1 workers`. It is the first thing every user of this tool sees and, on a default run, the only thing until they interrupt it. The other half of this, `1 seconds`, went with the duration flag in 0.4.0: the timeout is a `time.Duration` and formats itself (#17c)
- A SIGINT or SIGTERM arriving as the timeout expired killed the process with `panic: close of closed channel` and exit code 2. `Run` closed `s.done` in its signal branch and `timer()` closed the same channel from its own goroutine, with nothing coordinating the two, so both could run. The window is sub-millisecond and the process was exiting anyway — what it cost was a stack trace and a nonzero exit code rather than any lost work, but a supervisor reading exit codes could not distinguish that shutdown from a crash, and the panic surfaced in `timer()`, where no `recover` could have caught it. Both triggers now cancel one context, which cancels once no matter how many sources fire, and `timer()` is gone in favour of `context.WithTimeout`. In an isolated harness releasing both events from a common barrier, the old structure panicked in 98,028 of 100,000 attempts and the new one in none of them (#14)
- `signal.Notify` registered a handler that was never stopped, so it outlived the run that installed it. `signal.NotifyContext` replaces it and its `stop` is deferred (#14)
- `Run` can be called more than once on the same `Stressy`. The `done` channel was created in `New` and closed by `Run`, so a second call panicked immediately on the already-closed channel; the context that replaces it is built per call, and `Stressy` no longer holds shutdown state between runs (#14)

## [0.4.0] - 2026-08-03
### Added
- CI workflow running lint, formatting, `go vet`, module tidiness and verification, build, `go test -race` and govulncheck on every push to main and every pull request
- A `release-dryrun` CI job running `goreleaser check` and `goreleaser release --snapshot --clean` on every push and pull request. The release pipeline was the one part of the repository CI never touched: `.github/workflows/goreleaser.yml` fires only on a `v*` tag, so a change to `.goreleaser.yaml` or the `Dockerfile` was validated for the first time at the moment it published — the same asymmetry that let #13 sit unnoticed for eighteen months. It runs unfiltered rather than on `paths:`, because the case it exists for is a dependabot bump to an action or base image, where nobody thinks to check the release path and a filter would skip exactly that. The dry run covers the eleven cross-compiles, `-trimpath` and `mod_timestamp`, archive naming, checksums and both architecture image builds against the real `Dockerfile`. It does not cover the publishing half: snapshot mode skips the ghcr.io login, the image push, `docker_manifests` and release creation, so those stay verifiable only by pushing a real tag — cheaply, as a `vX.Y.Z-rcN` pre-release, which the `{{ if not .Prerelease }}` guards keep off `:latest` (#36)
- `.github/dependabot.yml` checking Go modules and GitHub Actions `uses:` refs weekly, so dependency security fixes arrive as pull requests instead of waiting for someone to notice an advisory. Each of those pull requests also runs CI, which is what keeps govulncheck scanning during quiet periods now that the nightly schedule is gone
- `.golangci.yml` configuring golangci-lint as the single Go gate for both formatting and linting
- Unit tests for the flag package (registration, environment binding, allowed-value validation, duration parsing in both spellings), for stress test configuration validation, and CLI-level tests covering flag/environment variable precedence

### Changed
- `--timeout` now takes a duration, so `-t 5m` and `STRESSY_TIMEOUT=90s` say what they look like they say. Bare seconds still work — `-t 60` is a minute, exactly as before — because `time.ParseDuration` rejects a unit-less number, so the two spellings cannot be confused for one another and no existing command line or environment variable had to change. This also closes the sharpest edge of the malformed-value bug below: `STRESSY_TIMEOUT=60s` was the natural thing to type and the one thing that silently produced an endless run (#26)
- The Docker image is now `FROM scratch` rather than Alpine, and runs as UID/GID `65532` instead of root. The `restricted` Pod Security Standard requires `runAsNonRoot: true`, and the kubelet will not start a container under that setting if the image would run as root — so every pod spec running the old image had to pin `runAsUser` as well. For a tool whose usual deployment target is a cluster that was a scheduling friction rather than an exploitation risk, but it was the sharper of the two. Going to `scratch` also retires the base image as a source of CVEs entirely: images are only rebuilt on a release tag, so any distribution base accumulated advisories between releases with nothing to rebuild it. The binary is built `CGO_ENABLED=0` and needs no userland. The trade-off is that there is no shell, so `docker exec` and `kubectl exec` into a running container no longer work (#21)
- `ghcr.io/felipeneuwald/stressy:latest` is now only published for full releases. Tagging a pre-release such as `v0.4.0-rc1` used to repoint `:latest` — the tag `docker run` resolves by default — at a release candidate, and publish it as a full GitHub release; both are now guarded (#23)
- Release binaries are built with `-trimpath` and stamped with the commit timestamp, so the same commit built twice produces the same artefact instead of embedding the builder's absolute paths and the wall-clock time of the release job (#23)
- Release and vulnerability scan workflows now build with the latest stable Go instead of pinning 1.23.6, a pin that never took effect
- Raised the go.mod Go directive to 1.26, which enables the cgroup-aware `GOMAXPROCS` default so containerised runs respect their CPU limit instead of sizing to the host
- Updated cobra 1.8.1 to 1.10.2, pflag 1.0.5 to 1.0.10, viper 1.19.0 to 1.21.0 and golang.org/x/crypto 0.46.0 to 0.54.0 — the first updates to arrive on their own rather than after someone noticed an advisory. Nothing a user touches changed: help text, version output, unknown-flag errors and invalid-value errors are byte-identical across the bump. Nor is this a security update in disguise; every x/crypto advisory fixed in that range is in `ssh`, and the only package this project imports is `bcrypt`, whose source is identical in both versions
- Release binaries are roughly a third smaller, 7.2 MB down to 4.8 MB when built with the release `-s -w` flags, because cobra 1.9 lets the linker drop `text/template` for commands that use the default help templates
- Six indirect dependencies dropped, 18 down to 12: viper 1.21 no longer pulls in the HCL, Java-properties and INI config decoders, `slog-shim`, `multierr` or `golang.org/x/exp`, and it moved `mapstructure` and YAML onto maintained forks. None of those decoders were ever reachable here — viper reads `STRESSY_WORKERS` and `STRESSY_TIMEOUT` from the environment and never opens a config file

### Removed
- The nightly vulnerability scan workflow. GitHub disables schedule-triggered workflows in public repositories after 60 days of inactivity, which had already happened, so it had been reporting nothing for months while appearing healthy. govulncheck now runs in CI on every push and pull request instead
- The `go mod tidy` release hook in `.goreleaser.yaml`. It ran after goreleaser's dirty-tree check, so a drift it corrected could not fail the release — it rewrote `go.mod` mid-build, shipped a dependency graph nobody had reviewed and stamped the binary `v0.3.3+dirty`, which is visible in `go version -m` on the published artefact. Tidiness is checked in CI instead, where a drift is a red pull request (#23)
- The `docker` entry in `.github/dependabot.yml`, which has no base image to watch now that the Dockerfile is `FROM scratch`. The commented-out block records what to restore if a base image ever returns (#21)
- The redundant `RUN chmod +x` layer in the Dockerfile, replaced by `COPY --chmod=0755`. Besides being a layer for nothing, a `RUN` forced QEMU emulation during the arm64 cross-build (#21)

### Fixed
- The MIT licence text now ships with the artefacts it covers. `.goreleaser.yaml` staged `LICENSE` into the Docker build context, but no `COPY` picked it up and `formats: binary` means there is no archive to carry it either — so it reached neither the images nor the binaries. The Dockerfile now copies it to `/LICENSE`, and it is uploaded as a release asset alongside the binaries (#23)
- `go install github.com/felipeneuwald/stressy@latest`, the README's primary installation instruction, failed with "module found (v0.3.3), but does not contain package": `package main` lived in `cmd/`, so the module root held no installable package and the path the README advertised could not resolve. The main package now sits at the module root, so that command works as written; `main: ./cmd` in `.goreleaser.yaml` moved with it. Release artefacts and images are unaffected — same binary, same name, same import path for every `internal/` package (#9)
- The README's build instruction, `go build ./cmd` followed by `./stressy`, produced a binary named `cmd` and then ran something that was not there. `go build` in the repo root now produces `stressy` (#9)
- A malformed environment variable (for example `STRESSY_TIMEOUT=abc`) was silently discarded, leaving the flag at zero and turning a bounded run into an indefinite one that exited 0; it is now rejected with a message naming the flag and the value
- gofmt formatting in `internal/flag/load.go` and `internal/stressy/stressy.go`

### Security
- Retired the Docker base image. Published `ghcr.io/felipeneuwald/stressy` images had been built on Alpine 3.19, which left support on 2025-11-01, so they carried a base layer that no longer received security fixes — and scanners flag an end-of-life release on its own regardless of which CVEs sit in it. That was first fixed by moving to Alpine 3.24; the image is now `FROM scratch` instead, which removes the class of problem rather than resetting its clock (see Changed). The binary is static and was never affected — this was always about the rest of the image, which no longer exists

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

[Unreleased]: https://github.com/felipeneuwald/stressy/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/felipeneuwald/stressy/compare/v0.3.3...v0.4.0
[0.3.3]: https://github.com/felipeneuwald/stressy/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/felipeneuwald/stressy/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/felipeneuwald/stressy/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/felipeneuwald/stressy/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/felipeneuwald/stressy/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/felipeneuwald/stressy/releases/tag/v0.1.0
