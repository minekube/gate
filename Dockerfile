# Build the manager binary
FROM golang:1.14 AS build

# Health probe client
RUN GRPC_HEALTH_PROBE_VERSION=v0.3.2 && \
    wget -qO/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd cmd/
COPY internal internal/
COPY pkg pkg/
COPY gate.go ./

# Build
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -o gate gate.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:latest
# Need this since gate is compiled with CGO_ENABLED=1
RUN apk add libc6-compat --no-cache
WORKDIR /gate
COPY --from=build /bin/grpc_health_probe bin/
COPY --from=build /workspace/gate .
ENV GATE_HEALTH_ENABLED=true
CMD ["./gate"]