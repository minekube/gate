import grpc
import json
from google.protobuf import json_format
from minekube.gate.v1 import gate_service_pb2 as gatepb
from minekube.gate.v1 import gate_service_pb2_grpc as gateapi


def main():
    # Create a gRPC channel
    channel = grpc.insecure_channel('localhost:8080')

    # Create a stub (client)
    stub = gateapi.GateServiceStub(channel)

    try:
        # List servers
        response = stub.ListServers(gatepb.ListServersRequest())
        # Convert to JSON and print
        json_response = json_format.MessageToDict(response)
        print(json.dumps(json_response, indent=2))

    except grpc.RpcError as e:
        print(f"RPC failed: {e}")

    finally:
        channel.close()


if __name__ == '__main__':
    main()
