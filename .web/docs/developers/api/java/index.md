---
title: "Gate Java API - Minecraft Proxy Development in Java"
description: "Build Minecraft proxy extensions with Gate Java API. Familiar Java development environment with comprehensive SDK."
---

# <img src="https://cdn.jsdelivr.net/gh/devicons/devicon/icons/java/java-original.svg" class="tech-icon" alt="Java" /> Java

Gate provides a Java API for integrating with your Java applications using gRPC. You can use the API to interact with Gate programmatically.

## Installation

First, configure your package manager to add the Buf registry. You only need to do this once:

::: code-group

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

:::

Then add the dependencies:

::: warning Latest Version Check
Make sure to check [buf.build/minekube/gate/sdks](https://buf.build/minekube/gate/sdks) for the latest versions of the dependencies.
:::

::: code-group

```xml [Maven]
<dependencies>
  <!-- Check latest version at https://buf.build/minekube/gate/sdks -->
  <dependency>
    <groupId>build.buf.gen</groupId>
    <artifactId>minekube_gate_protocolbuffers_java</artifactId>
    <version>28.3.0.2.20241118150055.50fffb007499</version>
  </dependency>
  <dependency>
    <groupId>build.buf.gen</groupId>
    <artifactId>minekube_gate_grpc_java</artifactId>
    <version>1.68.1.1.20241118150055.50fffb007499</version>
  </dependency>
</dependencies>
```

```kotlin [Gradle (Kotlin)]
dependencies {
  // Check latest version at https://buf.build/minekube/gate/sdks
  implementation("build.buf.gen:minekube_gate_protocolbuffers_java:28.3.0.2.20241118150055.50fffb007499")
  implementation("build.buf.gen:minekube_gate_grpc_java:1.68.1.1.20241118150055.50fffb007499")
}
```

```groovy [Gradle (Groovy)]
dependencies {
  // Check latest version at https://buf.build/minekube/gate/sdks
  implementation 'build.buf.gen:minekube_gate_protocolbuffers_java:28.3.0.2.20241118150055.50fffb007499'
  implementation 'build.buf.gen:minekube_gate_grpc_java:1.68.1.1.20241118150055.50fffb007499'
}
```

:::

## Usage Example

Here's a basic example of using the Gate Java API to connect to Gate and list servers:

```java
<!-- @include: ./src/main/java/com/example/Main.java -->
```

## Running the Example

1. Run Gate with the API enabled
2. Navigate to the [docs/developers/api/java](https://github.com/minekube/gate/tree/master/.web/docs/developers/api/java) directory
3. Run the following commands:

```bash
mvn compile
mvn exec:java -Dexec.mainClass="com.example.Main"

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
For more details on using gRPC with Java, check out the [gRPC Java Documentation](https://grpc.io/docs/languages/java/basics/).
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
