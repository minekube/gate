all: fmt vet mod lint

# Sync embedded config files from root directory
sync-configs:
	cp config.yml pkg/internal/configs/config.yml
	cp config-simple.yml pkg/internal/configs/config-simple.yml
	cp config-lite.yml pkg/internal/configs/config-lite.yml
	cp config-bedrock.yml pkg/internal/configs/config-bedrock.yml
	# Note: config-minimal.yml is maintained directly in pkg/internal/configs, not synced from root

# Build Gate with version information
build: sync-configs
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev-$$(git rev-parse --short HEAD 2>/dev/null || echo unknown)") && \
	echo "Building Gate version: $$VERSION" && \
	go build -ldflags="-s -w -X 'go.minekube.com/gate/pkg/version.Version=$$VERSION'" -o gate gate.go

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
	(cd .web && pnpm install && pnpm dev)

# Install gops & dependencies
pprof-gops-install:
	go install github.com/google/gops && \
	sudo apt install graphviz gv && \
	sudo apt install libcanberra-gtk-module

# Dump heap & show in browser
pprof-heap:
	curl -sK -v http://localhost:8080/debug/pprof/heap > /tmp/heap.out && \
	go tool pprof -http=:8081 /tmp/heap.out
