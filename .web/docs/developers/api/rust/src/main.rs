use minekube_gate_community_neoeinstein_prost::minekube::gate::v1::ListServersRequest;
use minekube_gate_community_neoeinstein_tonic::minekube::gate::v1::tonic::gate_service_client::GateServiceClient;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Create a runtime for async operations
    let rt = tokio::runtime::Runtime::new()?;
    rt.block_on(async {
        // Create a gRPC channel
        let channel = tonic::transport::Channel::from_static("http://localhost:8080")
            .connect()
            .await?;

        // Create the client
        let mut client = GateServiceClient::new(channel);

        // Make the request
        let request = tonic::Request::new(ListServersRequest {});
        let response = client.list_servers(request).await?;

        // Print the response
        println!("{:#?}", response.get_ref().servers);

        Ok(())
    })
}
