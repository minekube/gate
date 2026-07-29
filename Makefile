WASM_NATIVE_DIR := internal/wasm/runtime/native
WASM_FIXTURE_CORE := $(WASM_NATIVE_DIR)/target/wasm32-unknown-unknown/release/gate_wasm_fixture_guest.wasm
WASM_FIXTURE_COMPONENT := $(WASM_NATIVE_DIR)/artifacts/gate_wasm_fixture.component.wasm
WASM_RUST_EXAMPLE_DIR := .examples/wasm/rust
WASM_RUST_EXAMPLE_CORE := $(WASM_RUST_EXAMPLE_DIR)/target/wasm32-unknown-unknown/release/gate_wasm_rust_example.wasm
WASM_RUST_EXAMPLE_COMPONENT := $(WASM_RUST_EXAMPLE_DIR)/gate-rust-example.component.wasm

ifeq ($(OS),Windows_NT)
WASM_CARGO := rustup run 1.94.0-x86_64-pc-windows-gnu cargo
else
WASM_CARGO := cargo
endif

all: fmt vet mod lint

wasm-fixture-component:
	cd $(WASM_NATIVE_DIR) && $(WASM_CARGO) build -p gate-wasm-fixture-guest --release --target wasm32-unknown-unknown
	cd $(WASM_NATIVE_DIR) && $(WASM_CARGO) run -p gate-wasm-componentize --release -- \
		target/wasm32-unknown-unknown/release/gate_wasm_fixture_guest.wasm \
		artifacts/gate_wasm_fixture.component.wasm

wasm-native-lib:
	cd $(WASM_NATIVE_DIR) && $(WASM_CARGO) build -p gate-wasm-native --release

wasm-native-test: wasm-api-check wasm-fixture-component wasm-native-lib
	CGO_ENABLED=1 go test -count=1 -tags=wasm_native ./internal/wasm/runtime/native

wasm-rust-example-test: wasm-api-check wasm-native-lib
	$(WASM_CARGO) build --manifest-path $(WASM_RUST_EXAMPLE_DIR)/Cargo.toml --release --target wasm32-unknown-unknown
	cd $(WASM_NATIVE_DIR) && $(WASM_CARGO) run -p gate-wasm-componentize --release -- \
		../../../../$(WASM_RUST_EXAMPLE_CORE) \
		../../../../$(WASM_RUST_EXAMPLE_COMPONENT)
	CGO_ENABLED=1 go test -count=1 -tags='wasm_native wasm_example' \
		./internal/wasm/runtime/native -run TestPublicRustExampleLoadsAndInitializes

wasm-api-generate:
	go run ./internal/wasm/cmd/gate-wasm-gen generate -repo . -out internal/wasm/api -native-out internal/wasm/runtime/native/host/src/generated -public-out wasm/wit

wasm-api-check:
	go run ./internal/wasm/cmd/gate-wasm-gen check -repo . -out internal/wasm/api -native-out internal/wasm/runtime/native/host/src/generated -public-out wasm/wit

# Sync embedded config files from root directory
sync-configs:
	cp config.yml pkg/configs/config.yml
	cp config-simple.yml pkg/configs/config-simple.yml
	cp config-lite.yml pkg/configs/config-lite.yml
	cp config-bedrock.yml pkg/configs/config-bedrock.yml
	# Note: config-minimal.yml is maintained directly in pkg/configs, not synced from root

# Build Gate with version information
build: sync-configs wasm-native-lib
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev-$$(git rev-parse --short HEAD 2>/dev/null || echo unknown)") && \
	echo "Building Gate version: $$VERSION" && \
	CGO_ENABLED=1 go build -tags=wasm_native \
		-ldflags="-s -w -X 'go.minekube.com/gate/pkg/version.Version=$$VERSION'" -o gate gate.go

# Run tests
test: fmt vet
	sh .web/docs/public/build_tags_test.sh
	@case "$$(uname -s)" in \
		MINGW*|MSYS*|CYGWIN*) echo "Skipping installer shell tests on Windows";; \
		*) bash .web/docs/public/install_test.sh;; \
	esac
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
