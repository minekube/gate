---
title: "Gate Rust API - High-Performance Minecraft Proxy Development"
description: "Develop high-performance Minecraft proxy extensions with Gate Rust API. Memory-safe and fast proxy plugin development."
---

# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/rust/rust-original.svg" class="tech-icon" alt="Rust" /> Rust Client

Gate provides a Rust API for integrating with your Rust applications. You can use the API to interact with Gate programmatically using gRPC.

## Environment Setup

First, make sure you have Rust and Cargo installed. If not, install them using [rustup](https://rustup.rs/):

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

## Registry Configuration

1. Configure Cargo to use the Buf registry. Add the following to your `~/.cargo/config.toml`:

```toml
[registries.buf]
index = "sparse+https://buf.build/gen/cargo/"
credential-provider = "cargo:token"
```

2. Configure authentication (required even for public repositories):
   1. Go to [Gate SDKs on Buf](https://buf.build/minekube/gate/sdks)
   2. Select the Rust SDK
   3. Scroll down to generate a token
   4. Use the token to authenticate:
   ```bash
    cargo login --registry buf "Bearer YOUR_TOKEN"
   ```

## Installation

Add the Gate SDK to your project:

```bash
cargo add --registry buf minekube_gate_community_neoeinstein-prost
cargo add --registry buf minekube_gate_community_neoeinstein-tonic
cargo add tonic --features tls-roots # Enable the features we need
cargo add tokio --features macros,rt-multi-thread # Async runtime
```

This is the sample `Cargo.toml` file from the [`docs/developers/api/rust`](https://github.com/minekube/gate/tree/master/.web/docs/developers/api/rust) directory:

```toml
<!--@include: ./Cargo.toml -->
```

## Usage Example

Here's a basic example of using the Gate Rust API to connect to Gate and list servers:

::: code-group

```rust [src/main.rs]
<!--@include: ./src/main.rs -->
```

:::

## Running the Example

1. Make sure Gate is running with the API enabled
2. Run the example:

```bash
cargo run
[
    Server {
        name: "server1",
        address: "localhost:25566",
        players: 0,
    },
    Server {
        name: "server2",
        address: "localhost:25567",
        players: 0,
    },
    Server {
        name: "server3",
        address: "localhost:25568",
        players: 0,
    },
    Server {
        name: "server4",
        address: "localhost:25569",
        players: 0,
    },
]
```

::: info Learn More
Refer to the [Buf Blog](https://buf.build/blog/bsr-generated-sdks-for-rust) for more information about using the generated Rust SDKs.
:::

<style>
.tech-icon {
  width: 32px;
  height: 32px;
  display: inline-block;
  vertical-align: middle;
  margin-right: 12px;
  position: relative;
  top: -2px;
}
</style>
