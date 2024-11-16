# Gate HTTP API

Gate exposes a gRPC API for automating administrative tasks and extending Gate's functionality in languages other than Go. The API leverages modern technologies like Protocol Buffers, gRPC, and ConnectRPC, with schemas managed through buf.build. The API is primarily designed for:

- Automating server registration and management
- Extending Gate's functionality in non-Go languages
- Integrating Gate with other systems and tools

While the native Go library provides the most comprehensive access to Gate's features, the HTTP API offers a language-agnostic alternative for core administrative operations. Note that some advanced features available in the Go library may not be exposed through the API.

For Go applications, we recommend using Gate's native Go library as it provides the most complete and type-safe access to Gate's functionality. Check out [Introduction](/developers/) for more details.

## API Technology Glossary

### Protocol Buffers (Protobuf)
Protocol Buffers is Google's language-neutral, platform-neutral, extensible mechanism for serializing structured data. It provides a more efficient and type-safe alternative to formats like JSON, with built-in schema validation and backwards compatibility support.

### gRPC
gRPC is a modern, open-source remote procedure call (RPC) framework that can run anywhere. It enables client and server applications to communicate transparently and makes it easier to build connected systems. gRPC uses Protocol Buffers as its interface definition language.

### ConnectRPC
ConnectRPC is a slim RPC framework that supports both gRPC and HTTP/1.1 JSON, making it ideal for web browsers and HTTP API clients. It provides a more lightweight alternative to full gRPC implementations while maintaining compatibility with gRPC services.

### buf.build
buf.build is a modern Protocol Buffers ecosystem that provides tools for managing, versioning, and sharing Protocol Buffer schemas. It includes features like linting, breaking change detection, and a schema registry. Gate uses buf.build to maintain its API definitions and generate client libraries.
