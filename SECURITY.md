# Security Policy

## Reporting a vulnerability

Report privately through GitHub:
[**Report a vulnerability**](https://github.com/felipeneuwald/stressy/security/advisories/new).
That opens a draft advisory visible only to you and the maintainer, and it is
the preferred channel — please do not open a public issue for something you
believe is exploitable.

This is a single-maintainer project, so the honest commitment is best effort: an
acknowledgement within about a week, and a fix released as soon as one exists.
You will be credited in the advisory and the changelog unless you would rather
not be. If a report turns out to be out of scope (see below), you will get a
reply saying so and why, rather than silence.

## Supported versions

| Version               | Supported          |
| --------------------- | ------------------ |
| Latest release        | :white_check_mark: |
| Anything earlier      | :x:                |

Fixes ship in a new release. There are no patch backports to older tags, and
published container images are rebuilt only on a release tag — which is the
reason the image is `FROM scratch`: with no distribution base layer there is
nothing in it to accumulate advisories between releases.

If you are running stressy from a container image, run a released tag and update
it when a release goes out. `:latest` tracks full releases only; pre-release
tags such as `v0.4.0-rc1` publish under their own version and leave `:latest`
alone.

## What the attack surface actually is

Being specific about this saves everyone time. stressy takes two flags and two
environment variables, hashes a seven-byte constant in a loop, and prints two
lines. It opens no network sockets, reads no configuration file, parses no
untrusted input and writes nothing to disk. The binary is static, built
`CGO_ENABLED=0`, and the image runs as UID/GID `65532` with no shell in it.

So the realistic surface is not the program logic. It is:

- **The dependency graph.** Three direct dependencies — cobra, pflag and
  `golang.org/x/crypto` — plus cobra's `mousetrap`. Most of this project's
  security history is exactly this: `x/crypto` advisories arriving through
  dependabot. `govulncheck` runs in CI on every push to main and every pull
  request, and dependabot checks Go modules and GitHub Actions weekly, so each
  of those pull requests re-runs the scan even during a quiet period.
- **The published images and binaries.** Built by
  [`.github/workflows/goreleaser.yml`](.github/workflows/goreleaser.yml) from a
  tag, reproducibly (`-trimpath`, artefact mtimes pinned to the commit), with a
  `checksums.txt` published alongside them. Verify a download against it. The
  artefacts are not signed today; if that matters to you, say so in an issue.
- **The release pipeline itself.** A change to `.goreleaser.yaml` or the
  `Dockerfile` that weakens what ships is a security-relevant change, which is
  why CI rehearses a full release on every pull request rather than first
  exercising that path when a tag is pushed.

## What is not a vulnerability

- **stressy consuming all available CPU.** That is the entire purpose of the
  tool. Bound a run with `-t`, and constrain it with a cgroup limit
  (`docker run --cpus`, or `resources.limits.cpu` in a pod spec) — the worker
  count follows that limit, so the limit is the control.
- **Anything requiring the ability to run arbitrary commands on the host
  already.** Someone who can start stressy with the flags of their choosing can
  equally start anything else.
- **A CVE in a package that is present in the module graph but not reachable
  from this code.** Report it anyway if you are unsure — `govulncheck` reasons
  about reachability and a second opinion costs little — but expect the reply to
  say so.
