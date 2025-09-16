package api

import (
	"go.minekube.com/gate/pkg/edition/bedrock/geyser"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func PlayersToProto(p []proxy.Player) []*pb.Player {
	var players []*pb.Player
	for _, player := range p {
		players = append(players, PlayerToProto(player))
	}
	return players
}

func PlayerToProto(p proxy.Player) *pb.Player {
	player := &pb.Player{
		Id:       p.ID().String(),
		Username: p.Username(),
	}

	// Check if this is a Bedrock player by looking for the Floodgate username prefix
	// Bedrock players typically have usernames starting with a dot (.) or other prefix
	if bedrockData := extractBedrockData(p); bedrockData != nil {
		player.Bedrock = bedrockData
	}

	return player
}

// extractBedrockData attempts to extract Bedrock player data from a player.
// This integrates with Gate's Floodgate system to get real BedrockData.
func extractBedrockData(p proxy.Player) *pb.BedrockPlayerData {
	// Try to get the Geyser connection from the player's context
	geyserConn, isBedrock := geyser.FromContext(p.Context())
	if !isBedrock || geyserConn.BedrockData == nil {
		return nil // Not a Bedrock player or no Bedrock data available
	}

	bedrockData := geyserConn.BedrockData

	return &pb.BedrockPlayerData{
		Xuid:         bedrockData.Xuid,
		DeviceOs:     bedrockData.DeviceOS.String(),
		Language:     bedrockData.Language,
		UiProfile:    int32(bedrockData.UIProfile),
		InputMode:    int32(bedrockData.InputMode),
		BehindProxy:  bedrockData.Proxy,
		LinkedPlayer: bedrockData.LinkedPlayer,
	}
}

func ServersToProto(s []proxy.RegisteredServer) []*pb.Server {
	var servers []*pb.Server
	for _, server := range s {
		servers = append(servers, ServerToProto(server))
	}
	return servers
}

func ServerToProto(s proxy.RegisteredServer) *pb.Server {
	return &pb.Server{
		Name:    s.ServerInfo().Name(),
		Address: s.ServerInfo().Addr().String(),
		Players: int32(s.Players().Len()),
	}
}
