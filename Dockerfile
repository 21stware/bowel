# syntax=docker/dockerfile:1.7
#
# bowel without a window, for servers, NAS boxes and routers: `bowel daemon`
# with the mihomo core beside it, on distroless's glibc base. See
# docs/docker.md.
#
#   docker build -t bowel .
#   docker run -d --name bowel -p 7899:7899 -v bowel:/data bowel
#
# Every stage but the last runs on the build machine and cross-compiles,
# so a multi-arch build needs no emulation.

ARG GO_VERSION=1.27.3

# ---- the core and the geo data, which change less often than the code ----
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS core
ARG TARGETARCH
ARG TARGETVARIANT
WORKDIR /src
COPY scripts/fetch-core.sh scripts/
# amd64's "v1" build runs on every x86-64 CPU (the default needs AVX2);
# 32-bit ARM is v7.
RUN case "$TARGETARCH" in \
      amd64) export MIHOMO_VARIANT=v1 ;; \
      arm) export GOARCH=armv7 ;; \
    esac; \
    GOOS=linux GOARCH="${GOARCH:-$TARGETARCH}" ./scripts/fetch-core.sh \
 && mkdir -p /out/opt/bowel/bin \
 && mv resources/linux-*/bin/mihomo /out/opt/bowel/bin/mihomo \
 && mv resources/geo resources/licenses /out/opt/bowel/

# ---- bowel itself ----
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
ARG TARGETARCH
ARG TARGETVARIANT
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# The flags `mygo build` links (packageFlags in MyGo): Linux executables
# carry no metadata of their own, so the name and the version are these.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    version=$(sed -n 's/^ *"version": *"\([^"]*\)".*/\1/p' mygo.json | head -n1) \
 && m=github.com/egoist/mygo \
 && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -o /out/opt/bowel/bowel \
      -ldflags "-s -w -X $m.production=1 -X $m.packageName=bowel -X $m.packageVersion=$version -X $m.packageIdentifier=dev.moho.app" . \
 && mkdir -p /out/opt/bowel/licenses/bowel /out/data \
 && cp LICENSE PACKAGING-EXCEPTION.md /out/opt/bowel/licenses/bowel/

# ---- the image ----
# MyGo reaches system libraries through purego, so bowel links glibc (the
# core is static): base-nossl is glibc, CA certificates and zoneinfo, and
# no shell or package manager.
FROM gcr.io/distroless/base-nossl-debian12
COPY --from=build /out/ /
COPY --from=core /out/ /

ARG VERSION=dev
LABEL org.opencontainers.image.title="bowel" \
      org.opencontainers.image.description="bowel without a window: a mihomo proxy with routes, a terminal interface and an MCP server for agents" \
      org.opencontainers.image.source="https://github.com/21stware/bowel" \
      org.opencontainers.image.url="https://bowel.dev" \
      org.opencontainers.image.documentation="https://github.com/21stware/bowel/blob/main/docs/docker.md" \
      org.opencontainers.image.licenses="PolyForm-Noncommercial-1.0.0 AND GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}"

# BOWEL_CONTAINER: the defaults of a container (the proxy open to the LAN,
# no desktop to take over). MOHO_CORE: the core above, by name.
ENV PATH=/opt/bowel \
    HOME=/data \
    BOWEL_DATA_DIR=/data \
    BOWEL_CONTAINER=1 \
    MOHO_CORE=/opt/bowel/bin/mihomo

VOLUME ["/data"]
WORKDIR /data
# 7899: the mixed (HTTP and SOCKS5) proxy port. 7900: MCP over HTTP, when
# BOWEL_MCP_HTTP turns it on.
EXPOSE 7899 7900
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["bowel", "health"]
ENTRYPOINT ["bowel"]
CMD ["daemon"]
