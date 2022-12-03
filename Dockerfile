FROM golang:1.19 AS build

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

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o gate gate.go

# Move binary into final image
FROM gcr.io/distroless/static:nonroot AS app
COPY --from=build /workspace/gate /
CMD ["/gate"]
