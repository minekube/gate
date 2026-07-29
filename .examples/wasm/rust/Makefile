CORE := target/wasm32-unknown-unknown/release/gate_wasm_rust_example.wasm
COMPONENT := gate-rust-example.component.wasm

.PHONY: build check

build:
	cargo build --release --target wasm32-unknown-unknown
	wasm-tools component new $(CORE) -o $(COMPONENT)

check: build
	wasm-tools validate --features component-model $(COMPONENT)
	wasm-tools component wit $(COMPONENT) >/dev/null
