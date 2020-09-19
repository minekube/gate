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
COPY pkg/edition/java/internal internal/
COPY pkg pkg/
COPY gate.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o gate gate.go

# Final image
FROM alpine:latest
WORKDIR /gate
COPY --from=build /bin/grpc_health_probe bin/
COPY --from=build /workspace/gate .
ENV GATE_HEALTH_ENABLED=true
CMD ["./gate"]