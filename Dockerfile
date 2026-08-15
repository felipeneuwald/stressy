# scratch, not a distribution base: the binary is static (CGO_ENABLED=0) and
# images are only rebuilt on a tag, so a base layer would just accumulate CVEs.
FROM scratch

LABEL org.opencontainers.image.source="https://github.com/felipeneuwald/stressy"
LABEL org.opencontainers.image.description="A lightweight CPU stress test tool"
LABEL org.opencontainers.image.licenses="MIT"

# The build context is staged by goreleaser, not by this repository: `stressy`
# is the binary it cross-compiled for the platform being built, and LICENSE is
# an `extra_files` entry in .goreleaser.yaml. A bare `docker build .` at the
# repository root copies whatever `go build` last left there instead, which is
# for one platform and is not a release; `goreleaser release --snapshot --clean`
# is how you build these images locally (#128).
#
# --chmod rather than RUN chmod, which scratch cannot run at all.
COPY --chmod=0755 stressy /usr/local/bin/stressy

# MIT requires the licence text to travel with the binary.
COPY LICENSE /LICENSE

# Numeric because scratch has no /etc/passwd, and baked in so a pod meeting the
# `restricted` Pod Security Standard does not have to pin runAsUser itself.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/stressy"]
