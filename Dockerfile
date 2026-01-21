FROM --platform=$BUILDPLATFORM golang:1.25.6 AS build

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

# Move binary into final image (default Gate image - distroless)
FROM --platform=$BUILDPLATFORM gcr.io/distroless/static-debian12 AS gate
COPY --from=build /workspace/gate /
ENTRYPOINT ["/gate"]

# Move binary into final image (jre variant Gate image - temurin-25-jre-alpine)
FROM --platform=$BUILDPLATFORM eclipse-temurin:25.0.1_8-jre-alpine AS jre
COPY --from=build /workspace/gate /usr/local/bin/gate
ENV PATH=/opt/java/openjdk/bin:$PATH
ENTRYPOINT ["/usr/local/bin/gate"]
