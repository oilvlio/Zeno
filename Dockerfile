# syntax=docker/dockerfile:1
# Builder versions match the CI toolchains exactly, so the shipped binary and
# assets are produced by the runtimes that passed the full test suite. Digest
# pins keep rebuild inputs immutable; version upgrades update CI and these two
# stages together. Published images include provenance and SBOM attestations.

FROM --platform=$BUILDPLATFORM node:26.7.0-bookworm-slim@sha256:cd565714d4da3e84bfd341e31448f81d47c6362198f152345297c9c1154e6341 AS web-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
ARG VERSION=dev
ENV VITE_BUILD_ID=${VERSION}
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web-builder /src/web/dist ./web/dist
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
# ca-certificates and tzdata are data files only: the Go binary reads the CA
# bundle for outbound TLS (exchange-rate refresh, notification webhooks) and
# the zone files for TZ-aware billing windows. None of this leaves the build
# stage; the runtime image stays dependency-free. /out/data gives the final
# stage a pre-owned data directory since scratch has no mkdir.
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/* \
  && mkdir -p /out/data \
  && if [ "${TARGETARCH}/${TARGETVARIANT}" = "arm/v6" ]; then export GOARM=6; fi \
  && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-s -w" -o /out/zeno-controller ./cmd/controller

# Runtime is FROM scratch: the controller is a static Go binary
# (CGO_ENABLED=0, pure-Go SQLite via modernc.org/sqlite) and the dashboard is
# static files, so no distribution userland ships. What lands here is the
# binary, the web assets, two data files (CA bundle, zoneinfo), and docs.
# Deliberately absent: shell, curl, ping.
# - Readiness is polled from the host (install.sh wait_ready hits /ready);
#   there is no in-container curl healthcheck.
# - `ping` only serves the opt-in -collect-local preview collector, which is
#   off by default. Enabling it needs a `ping` binary in PATH (extend this
#   image or bind-mount iputils); without it ping probes fail closed as
#   connect errors and agent-reported data is unaffected.
FROM scratch
ARG VERSION=dev
ARG REVISION=unknown
ARG ZENO_UID=10001
ARG ZENO_GID=10001
LABEL org.opencontainers.image.title="Zeno" \
  org.opencontainers.image.description="Lightweight self-hosted server monitor" \
  org.opencontainers.image.source="https://github.com/shui1iao/Zeno" \
  org.opencontainers.image.url="https://github.com/shui1iao/Zeno" \
  org.opencontainers.image.licenses="MIT" \
  org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${REVISION}"
COPY --from=go-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go-builder --chown=${ZENO_UID}:${ZENO_GID} /out/zeno-controller /usr/local/bin/zeno-controller
COPY --from=web-builder --chown=${ZENO_UID}:${ZENO_GID} /src/web/dist /opt/zeno/web
COPY --chown=${ZENO_UID}:${ZENO_GID} LICENSE THIRD_PARTY_NOTICES.txt /usr/share/doc/zeno/
COPY --from=go-builder --chown=${ZENO_UID}:${ZENO_GID} /out/data /data
USER ${ZENO_UID}:${ZENO_GID}
WORKDIR /opt/zeno
ENV TZ=Asia/Shanghai
EXPOSE 18980
ENTRYPOINT ["/usr/local/bin/zeno-controller"]
CMD ["-addr", "0.0.0.0:18980", "-web-dir", "/opt/zeno/web", "-db", "/data/zeno.db", "-admin-token-file", "/run/secrets/zeno_admin_token", "-agent-token-file", "/run/secrets/zeno_agent_token", "-notification-authority-key-file", "/run/secrets/zeno_notification_authority"]
