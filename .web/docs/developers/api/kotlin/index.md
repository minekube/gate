---
title: 'Gate Kotlin API - Modern Minecraft Proxy Development'
description: 'Develop Minecraft proxy extensions with Gate Kotlin API. Modern JVM language with concise syntax and Java interoperability.'
---

# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/kotlin/kotlin-original.svg" class="tech-icon" alt="Kotlin" /> Kotlin

Gate provides a Kotlin API for integrating with your Kotlin applications using either Connect-RPC or gRPC. You can use the API to interact with Gate programmatically.

## Installation

First, configure your package manager to add the Buf registry. You only need to do this once:

::: code-group

```kotlin [Gradle (Kotlin)]
// build.gradle.kts
repositories {
    mavenCentral()
    maven {
        name = "buf"
        url = uri("https://buf.build/gen/maven")
    }
}
```

```groovy [Gradle (Groovy)]
// build.gradle
repositories {
    mavenCentral()
    maven {
        name = 'buf'
        url 'https://buf.build/gen/maven'
    }
}
```

```xml [Maven]
<!-- pom.xml -->
<repositories>
    <repository>
        <name>Buf Maven Repository</name>
        <id>buf</id>
        <url>https://buf.build/gen/maven</url>
        <releases>
            <enabled>true</enabled>
        </releases>
        <snapshots>
            <enabled>false</enabled>
        </snapshots>
    </repository>
</repositories>
```

:::

Then add the dependencies. You can choose between Connect-RPC (recommended) or gRPC:

::: warning Latest Version Check
Make sure to check [buf.build/minekube/gate/sdks](https://buf.build/minekube/gate/sdks) for the latest versions of the dependencies.
:::

::: code-group

```kotlin [Gradle (Kotlin) - Connect]
dependencies {
    // Connect-RPC for Kotlin (Recommended)
    implementation("build.buf.gen:minekube_gate_connectrpc_kotlin:0.7.1.1.20241118150055.50fffb007499")
    implementation("com.connectrpc:connect-kotlin:0.7.1")
}
```

```groovy [Gradle (Groovy) - Connect]
dependencies {
    // Connect-RPC for Kotlin (Recommended)
    implementation 'build.buf.gen:minekube_gate_connectrpc_kotlin:0.7.1.1.20241118150055.50fffb007499'
    implementation 'com.connectrpc:connect-kotlin:0.7.1'
}
```

```xml [Maven - Connect]
<dependencies>
    <!-- Connect-RPC for Kotlin (Recommended) -->
    <dependency>
        <groupId>build.buf.gen</groupId>
        <artifactId>minekube_gate_connectrpc_kotlin</artifactId>
        <version>0.7.1.1.20241118150055.50fffb007499</version>
    </dependency>
    <dependency>
        <groupId>com.connectrpc</groupId>
        <artifactId>connect-kotlin</artifactId>
        <version>0.7.1</version>
    </dependency>
</dependencies>
```

```kotlin [Gradle (Kotlin) - gRPC]
dependencies {
    // gRPC for Kotlin
    implementation("build.buf.gen:minekube_gate_grpc_kotlin:1.4.1.1.20241118150055.50fffb007499")
    implementation("io.grpc:grpc-kotlin-stub:1.4.1")
    implementation("io.grpc:grpc-protobuf:1.68.1")
    implementation("io.grpc:grpc-netty-shaded:1.68.1")
}
```

```groovy [Gradle (Groovy) - gRPC]
dependencies {
    // gRPC for Kotlin
    implementation 'build.buf.gen:minekube_gate_grpc_kotlin:1.4.1.1.20241118150055.50fffb007499'
    implementation 'io.grpc:grpc-kotlin-stub:1.4.1'
    implementation 'io.grpc:grpc-protobuf:1.68.1'
    implementation 'io.grpc:grpc-netty-shaded:1.68.1'
}
```

```xml [Maven - gRPC]
<dependencies>
    <!-- gRPC for Kotlin -->
    <dependency>
        <groupId>build.buf.gen</groupId>
        <artifactId>minekube_gate_grpc_kotlin</artifactId>
        <version>1.4.1.1.20241118150055.50fffb007499</version>
    </dependency>
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-kotlin-stub</artifactId>
        <version>1.4.1</version>
    </dependency>
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-protobuf</artifactId>
        <version>1.68.1</version>
    </dependency>
    <dependency>
        <groupId>io.grpc</groupId>
        <artifactId>grpc-netty-shaded</artifactId>
        <version>1.68.1</version>
    </dependency>
</dependencies>
```

:::

## Usage Example

Here's a basic example of using the Gate Kotlin API to connect to Gate and list servers:

::: code-group

```kotlin [Connect]
<!--@include: ./src/main/kotlin/com/example/connect/ConnectExample.kt -->
```

```kotlin [gRPC]
<!--@include: ./src/main/kotlin/com/example/grpc/GrpcExample.kt -->
```

:::

## Running the Example

1. Run Gate with the API enabled
2. Navigate to the [docs/developers/api/kotlin](https://github.com/minekube/gate/tree/master/.web/docs/developers/api/kotlin) directory
3. Initialize the Gradle wrapper (only needed once):

```bash
gradle wrapper
```

4. Run one of the following commands:

```bash
# For Connect example (recommended)
./gradlew runConnect

# For gRPC example
./gradlew runGrpc

# Example output:
servers {
  name: "server3"
  address: "localhost:25568"
}
servers {
  name: "server4"
  address: "localhost:25569"
}
servers {
  name: "server1"
  address: "localhost:25566"
}
servers {
  name: "server2"
  address: "localhost:25567"
}
```

::: info Learn More
For more details on using ConnectRPC with Kotlin, check out the [ConnectRPC Documentation](https://connectrpc.com/docs/kotlin/using-clients).
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
