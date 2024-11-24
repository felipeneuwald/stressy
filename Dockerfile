FROM alpine:3.19

LABEL org.opencontainers.image.source="https://github.com/felipeneuwald/stressy"
LABEL org.opencontainers.image.description="A simple stress test tool"
LABEL org.opencontainers.image.licenses="MIT"

COPY stressy /usr/local/bin/
RUN chmod +x /usr/local/bin/stressy

ENTRYPOINT ["/usr/local/bin/stressy"]
