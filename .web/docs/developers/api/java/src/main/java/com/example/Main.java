package com.example;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import build.buf.gen.minekube.gate.v1.*;

public class Main {
    public static void main(String[] args) {
        try {
            // Create a gRPC channel
            ManagedChannel channel = ManagedChannelBuilder
                .forAddress("localhost", 8080)
                .usePlaintext()
                .build();

            // Create a blocking stub
            GateServiceGrpc.GateServiceBlockingStub stub = GateServiceGrpc.newBlockingStub(channel);

            // List all servers
            ListServersResponse response = stub.listServers(ListServersRequest.getDefaultInstance());

            // Print protobuf response
            System.out.println(response);

            // Shutdown the channel
            channel.shutdown();
        } catch (Exception e) {
            System.err.println("Make sure Gate is running with the API enabled");
            e.printStackTrace();
        }
    }
} 
