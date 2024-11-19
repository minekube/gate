package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"buf.build/gen/go/minekube/gate/connectrpc/go/minekube/gate/v1/gatev1connect"
	gatev1 "buf.build/gen/go/minekube/gate/protocolbuffers/go/minekube/gate/v1"
	"connectrpc.com/connect"
)

// main is an example of how to use the ListServers method.
func main() {
	ctx := context.Background()
	
	client := gatev1connect.NewGateServiceClient(
		http.DefaultClient, 
		"http://localhost:8080",
	)

	req := connect.NewRequest(&gatev1.ListServersRequest{})
	res, err := client.ListServers(ctx, req)
	if err != nil {
		log.Fatalln("make sure Gate is running with the API enabled", err)
	}

	j, _ := json.MarshalIndent(res.Msg, "", "  ")
	println(string(j))
}
