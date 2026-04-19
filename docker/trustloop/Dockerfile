FROM golang:1.26.2-bookworm AS builder

ARG NODE_VERSION=20.19.0
ARG VERSION=dev-build
ARG TARGETARCH

WORKDIR /src

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential ca-certificates curl libsqlite3-dev make pkg-config xz-utils \
    && rm -rf /var/lib/apt/lists/*

RUN case "${TARGETARCH}" in \
        amd64) NODE_ARCH="x64" ;; \
        arm64) NODE_ARCH="arm64" ;; \
        *) echo "Unsupported target architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" -o /tmp/node.tar.xz \
    && tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
    && rm /tmp/node.tar.xz

COPY . /src/

RUN make build DYNAMIC=1 LOCAL=1 VERSION="${VERSION}" \
    && test -f /src/tmp/bifrost-http \
    && cp /src/tmp/bifrost-http /src/bifrost-http \
    && cd plugins/toolinjector \
    && make build-test-plugin \
    && cd /src/plugins/openrouterrouting \
    && make build-test-plugin

FROM debian:bookworm-slim

ENV APP_HOST=0.0.0.0
ENV APP_PORT=8080
ENV APP_DIR=/app/data
ENV LOG_LEVEL=info
ENV LOG_STYLE=json

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libsqlite3-0 wget \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 1000 --user-group appuser \
    && mkdir -p /home/appuser/.config/bifrost \
    && mkdir -p /app/data \
    && chown -R appuser:appuser /app /home/appuser/.config

COPY --from=builder /src/bifrost-http /app/main
COPY --from=builder /src/transports/docker-entrypoint.sh /app/docker-entrypoint.sh
COPY --from=builder /src/plugins/toolinjector/build/tool_injector.so /opt/tool_injector.so
COPY --from=builder /src/plugins/openrouterrouting/build/openrouter_provider_routing.so /opt/openrouter_provider_routing.so

RUN chmod +x /app/docker-entrypoint.sh \
    && chmod 0644 /opt/tool_injector.so /opt/openrouter_provider_routing.so

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 -O /dev/null http://127.0.0.1:${APP_PORT}/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/main"]
