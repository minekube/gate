# API Technology Glossary

This glossary provides a brief overview of the key technologies used in Gate's API.

## Protocol Buffers (Protobuf)

Protocol Buffers is Google's language-neutral, platform-neutral, extensible mechanism for serializing structured data. It provides a more efficient and type-safe alternative to formats like JSON, with built-in schema validation and backwards compatibility support.

## gRPC

gRPC is a modern, open-source remote procedure call (RPC) framework that can run anywhere. It enables client and server applications to communicate transparently and makes it easier to build connected systems. gRPC uses Protocol Buffers as its interface definition language.

## ConnectRPC

ConnectRPC is a slim RPC framework that supports both gRPC and HTTP/1.1 JSON, making it ideal for web browsers and HTTP API clients. It provides a more lightweight alternative to full gRPC implementations while maintaining compatibility with gRPC services.

## buf.build

buf.build is a modern Protocol Buffers ecosystem that provides tools for managing, versioning, and sharing Protocol Buffer schemas. It includes features like linting, breaking change detection, and a schema registry. Gate uses buf.build to maintain its API definitions and automatically generate client libraries in multiple programming languages.
