# Security Policy

## Reporting a vulnerability

Report privately: [**Report a vulnerability**](https://github.com/felipeneuwald/stressy/security/advisories/new).
Please do not open a public issue for something you believe is exploitable.
Best effort from a single maintainer: a reply within about a week, and credit.

## Supported versions

The latest release, with no backports to older tags; images are rebuilt only on
a release tag, and `:latest` tracks full releases only.

## What the attack surface actually is

stressy takes three flags and three environment variables, hashes a seven-byte
constant in a loop, and prints a handful of lines to stdout: no sockets, no
config file, no untrusted input, nothing to disk. So the realistic surface is:

- **The dependency graph.** Three direct dependencies. `govulncheck` runs in CI
  on every push to main and every pull request, and dependabot checks weekly.
- **The published images and binaries.** Verify a download against `checksums.txt`.
- **The release pipeline itself.** `.goreleaser.yaml` and the `Dockerfile`
  decide what ships, so a change to either is security-relevant.

## What is not a vulnerability

- **stressy consuming all available CPU.** The entire purpose of the tool.
- **Anything that already requires running arbitrary commands on the host.**
- **A CVE in the module graph that this code cannot reach.** Ask if unsure.
