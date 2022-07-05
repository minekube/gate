FROM golang:1.18 AS build

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd cmd/
COPY pkg pkg/
COPY gate.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o gate gate.go

# Final image
FROM alpine:latest
WORKDIR /gate
COPY --from=build /workspace/gate .
CMD ["./gate"]