all: fmt vet mod lint

# Get the current git tag for versioning
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X go.minekube.com/gate/pkg/telemetry.Version=$(VERSION)

# Build the binary with version information
build:
	go build -ldflags "$(LDFLAGS)" ./cmd/gate

# Run tests
test: fmt vet
	go test ./...

# Run go fmt against code
fmt:
	go fmt ./...

# Run go fmt against code
mod:
	go mod tidy && go mod verify

# Run go vet against code
vet:
	go vet ./...

# Run golangci-lint against code
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

# Serve the docs website locally and auto on changes
dev-docs:
	cd .web && yarn install && yarn dev

# Install gops & dependencies
pprof-gops-install:
	go install github.com/google/gops && \
	sudo apt install graphviz gv && \
	sudo apt install libcanberra-gtk-module

# Dump heap & show in browser
pprof-heap:
	curl -sK -v http://localhost:8080/debug/pprof/heap > /tmp/heap.out && \
	go tool pprof -http=:8081 /tmp/heap.out
