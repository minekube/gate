package com.example.connect

import build.buf.gen.minekube.gate.v1.GateServiceClient
import build.buf.gen.minekube.gate.v1.ListServersRequest
import com.connectrpc.ConnectException
import com.connectrpc.ProtocolClientConfig
import com.connectrpc.extensions.GoogleJavaProtobufStrategy
import com.connectrpc.impl.ProtocolClient
import com.connectrpc.okhttp.ConnectOkHttpClient
import com.connectrpc.protocols.NetworkProtocol
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient

fun main() = runBlocking {
    try {
        // Create a Connect client
        val client = ProtocolClient(
            httpClient = ConnectOkHttpClient(OkHttpClient()),
            ProtocolClientConfig(
                host = "http://localhost:8080",
                serializationStrategy = GoogleJavaProtobufStrategy(),
                networkProtocol = NetworkProtocol.CONNECT,
            ),
        )

        // Create the service client
        val gateService = GateServiceClient(client)

        // List all servers
        val request = ListServersRequest.newBuilder().build()
        val response = gateService.listServers(request)
        println(response.toString())

    } catch (e: ConnectException) {
        System.err.println("Make sure Gate is running with the API enabled")
        e.printStackTrace()
    }
}
