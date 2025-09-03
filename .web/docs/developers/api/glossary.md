---
title: "Gate API Glossary - Minecraft Proxy Terms & Definitions"
description: "Comprehensive glossary of Gate API terms and concepts. Understand Minecraft proxy terminology, API concepts, and development vocabulary."
---

# API Technology Glossary

This glossary provides a brief overview of the key technologies used in Gate's API.

## Protocol Buffers (Protobuf)

Protocol Buffers is Google's language-neutral, platform-neutral, extensible mechanism for serializing structured data. It provides a more efficient and type-safe alternative to formats like JSON, with built-in schema validation and backwards compatibility support.

## gRPC

gRPC is a modern, open-source remote procedure call (RPC) framework that runs over HTTP/2. It enables client and server applications to communicate transparently and makes it easier to build connected systems. gRPC uses Protocol Buffers as its interface definition language and leverages HTTP/2's features like multiplexing and header compression for efficient communication.

## ConnectRPC

ConnectRPC is a slim RPC framework that supports both gRPC and HTTP/1.1 JSON, making it ideal for web browsers and HTTP API clients. It provides a more lightweight alternative to full gRPC implementations while maintaining compatibility with gRPC services.

## buf.build

buf.build is a modern Protocol Buffers ecosystem that provides tools for managing, versioning, and sharing Protocol Buffer schemas. It includes features like linting, breaking change detection, and a schema registry. Gate uses buf.build to maintain its API definitions and automatically generate client libraries in multiple programming languages.

## HTTP/1.1

HTTP/1.1 is the most widely used version of the HTTP protocol that enables client-server communication on the web. Gate's API supports HTTP/1.1 through ConnectRPC, allowing for broad compatibility with web browsers and standard HTTP clients.

## JSON (JavaScript Object Notation)

JSON is a lightweight, text-based data interchange format that is easy for humans to read and write and easy for machines to parse and generate. While Gate primarily uses Protocol Buffers for efficiency, it also supports JSON encoding through ConnectRPC for better web compatibility.

## SDK (Software Development Kit)

An SDK is a collection of tools, libraries, documentation, and examples that developers use to create applications for specific platforms or programming languages. Gate provides official SDKs for multiple languages including TypeScript, Python, Go, Rust, Kotlin, and Java through buf.build. See https://buf.build/minekube/gate/sdks for more information and available SDKs.