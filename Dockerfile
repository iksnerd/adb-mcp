# syntax=docker/dockerfile:1
#
# Image for the Glama listing check: the server only has to start and answer an
# introspection request (tools/list) over stdio. adb is installed anyway so the
# image isn't lying about what it can do — without it the server starts fine and
# every device tool fails at call time instead.
#
# No JDK or Android SDK needed at build time: the only go:embed targets are the
# markdown guides, and the accessibility bridge APK is fetched at runtime by
# internal/bridgeupdate rather than compiled in.

# --platform=$BUILDPLATFORM keeps the toolchain native and cross-compiles via
# GOARCH, so a multi-arch build doesn't drag in QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Module cache layer: unchanged deps mean this survives edits to the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -trimpath -ldflags="-s -w" -o /out/adb-mcp ./cmd/adb-mcp

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends adb ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# The MCP registry proves ownership of an OCI package by matching this label
# against the "name" in server.json. They must stay identical.
LABEL io.modelcontextprotocol.server.name="io.github.iksnerd/adb-mcp"

COPY --from=build /out/adb-mcp /usr/local/bin/adb-mcp

# stdio transport: no ports to expose, no daemon to supervise. The MCP client
# owns the process lifetime, so exec form matters — the binary must be PID 1 and
# receive signals directly.
ENTRYPOINT ["adb-mcp"]
