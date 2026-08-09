# scratch, not a distribution base: the binary is static (CGO_ENABLED=0) and
# images are only rebuilt on a tag, so a base layer would just accumulate CVEs.
FROM scratch

LABEL org.opencontainers.image.source="https://github.com/felipeneuwald/stressy"
LABEL org.opencontainers.image.description="A lightweight CPU stress test tool"
LABEL org.opencontainers.image.licenses="MIT"

# --chmod rather than RUN chmod, which scratch cannot run at all.
COPY --chmod=0755 stressy /usr/local/bin/stressy

# MIT requires the licence text to travel with the binary.
COPY LICENSE /LICENSE

# Numeric because scratch has no /etc/passwd, and baked in so a pod meeting the
# `restricted` Pod Security Standard does not have to pin runAsUser itself.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/stressy"]
