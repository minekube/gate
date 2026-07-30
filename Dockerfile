FROM --platform=$BUILDPLATFORM golang:1.26 AS build

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.sum ./

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd ./cmd
COPY pkg ./pkg
COPY gate.go ./

# Automatically provided by the buildkit
ARG TARGETOS TARGETARCH

# Build
ARG VERSION=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w -X 'go.minekube.com/gate/pkg/version.Version=${VERSION}'" -a -o gate gate.go

# The arm64 geyserlite executable is dynamically linked and needs zlib.
# Stage the target-platform library so managed Bedrock also works in the
# minimal distroless image.
FROM debian:bookworm-slim AS runtime-deps
RUN mkdir -p /runtime-libs \
    && cp -L /usr/lib/*-linux-gnu/libz.so.1 /runtime-libs/libz.so.1

# Move binary into final image (default Gate image - distroless)
# NOTE: We use distroless/base (glibc) instead of distroless/static because the
# Gate binary is dynamically linked. The geyserlite dependency (used by the
# managed Bedrock mode) pulls in github.com/ebitengine/purego, whose no-cgo
# Linux path declares //go:cgo_import_dynamic for libdl.so.2. That forces the
# binary to depend on the glibc dynamic loader (/lib64/ld-linux-x86-64.so.2)
# even with CGO_ENABLED=0, so a static-only base would crash at startup with
# "exec /gate: no such file or directory".
FROM gcr.io/distroless/base-debian12 AS gate
COPY --from=build /workspace/gate /
COPY --from=runtime-deps /runtime-libs/libz.so.1 /usr/lib/libz.so.1
ENV XDG_CACHE_HOME=/var/cache/gate
VOLUME ["/var/cache/gate"]
ENTRYPOINT ["/gate"]

# Move binary into final image (jre variant Gate image - temurin-25-jre)
# Must be a glibc-based JRE (not the alpine/musl variant) for the same reason
# as above: the dynamically linked Gate binary needs the glibc loader.
FROM eclipse-temurin:25.0.3_9-jre AS jre
COPY --from=build /workspace/gate /usr/local/bin/gate
ENV PATH=/opt/java/openjdk/bin:$PATH
ENV XDG_CACHE_HOME=/var/cache/gate
VOLUME ["/var/cache/gate"]
ENTRYPOINT ["/usr/local/bin/gate"]
