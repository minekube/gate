package com.example.grpc

import io.grpc.ManagedChannelBuilder
import build.buf.gen.minekube.gate.v1.GateServiceGrpcKt
import build.buf.gen.minekube.gate.v1.ListServersRequest
import kotlinx.coroutines.runBlocking

fun main(): Unit = runBlocking {
    try {
        // Create a gRPC channel
        val channel = ManagedChannelBuilder
            .forAddress("localhost", 8080)
            .usePlaintext()
            .build()

        // Create the service client
        val stub = GateServiceGrpcKt.GateServiceCoroutineStub(channel)

        // List all servers
        val response = stub.listServers(ListServersRequest.getDefaultInstance())
        println(response)

        // Shutdown the channel
        channel.shutdown()

    } catch (e: Exception) {
        System.err.println("Make sure Gate is running with the API enabled")
        e.printStackTrace()
    }
}
