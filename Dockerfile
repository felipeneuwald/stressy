# scratch, not a distribution base image. The binary is built CGO_ENABLED=0
# (.goreleaser.yaml), so it is fully static and needs no userland: no libc, no
# CA certificates — stressy opens no sockets — and no shell.
#
# That last one is the trade-off, and it is deliberate. Images are only rebuilt
# on a release tag, so any distribution base would accumulate CVEs between
# releases with nothing to rebuild it; scratch has no base layer to accumulate
# them in. The cost is that `kubectl exec` into a running stressy container is
# impossible. Reach for a shell-bearing base only if that becomes a real need —
# and if you do, digest-pin it and restore the `docker` entry in
# .github/dependabot.yml, which has nothing to update while this stays scratch.
FROM scratch

LABEL org.opencontainers.image.source="https://github.com/felipeneuwald/stressy"
LABEL org.opencontainers.image.description="A simple stress test tool"
LABEL org.opencontainers.image.licenses="MIT"

# --chmod rather than a RUN chmod: a RUN layer on scratch is impossible, and
# even on a distribution base it forced QEMU emulation during the arm64
# cross-build for no benefit. Requires BuildKit, which `use: buildx` in
# .goreleaser.yaml guarantees.
COPY --chmod=0755 stressy /usr/local/bin/stressy

# MIT requires the licence text to travel with the binary. goreleaser stages it
# into the build context via `extra_files`; before this COPY existed, that
# staging went nowhere and the images shipped no licence at all.
COPY LICENSE /LICENSE

# Numeric on purpose: scratch has no /etc/passwd for a name to resolve against.
# Kubernetes Pod Security Standards `restricted` requires runAsNonRoot, which a
# root-only image cannot satisfy — and a cluster is where this tool most often
# runs. 65532 is the conventional "nonroot" UID.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/stressy"]
