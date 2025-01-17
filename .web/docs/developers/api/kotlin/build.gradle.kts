plugins {
    kotlin("jvm") version "1.9.25"
    application
}

repositories {
    mavenCentral()
    maven {
        name = "buf"
        url = uri("https://buf.build/gen/maven")
    }
}

val grpcVersion = "1.69.1"
val grpcKotlinVersion = "1.4.1"
val connectVersion = "0.7.1"
val protobufVersion = "4.29.3"

dependencies {
    // Kotlin
    implementation(kotlin("stdlib"))
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.10.1")

    // Connect-RPC
    implementation("build.buf.gen:minekube_gate_connectrpc_kotlin:${connectVersion}.1.20241118150055.50fffb007499")

    // gRPC
    implementation("build.buf.gen:minekube_gate_grpc_kotlin:${grpcKotlinVersion}.1.20241118150055.50fffb007499")
    implementation("io.grpc:grpc-kotlin-stub:$grpcKotlinVersion")
    implementation("io.grpc:grpc-protobuf:$grpcVersion")
    implementation("io.grpc:grpc-netty-shaded:$grpcVersion")
    
    // Protobuf
    implementation("com.google.protobuf:protobuf-java:$protobufVersion")
}

// Configure multiple main classes
application {
    mainClass.set("com.example.ConnectExampleKt") // Default main class
}

// Create tasks for running each example
tasks.register<JavaExec>("runConnect") {
    classpath = sourceSets["main"].runtimeClasspath
    mainClass.set("com.example.connect.ConnectExampleKt")
}

tasks.register<JavaExec>("runGrpc") {
    classpath = sourceSets["main"].runtimeClasspath
    mainClass.set("com.example.grpc.GrpcExampleKt")
} 
