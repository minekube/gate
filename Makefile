all: fmt vet mod lint

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
	golangci-lint run



updatedocsy:
	git submodule update --depth 1 --init --recursive site/themes/docsy

# Install gops & dependency for profiling
install-gops:
	go install github.com/google/gops && \
	sudo apt install graphviz gv && \
	sudo apt install libcanberra-gtk-module libcanberra-gtk3-module