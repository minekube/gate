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

	// Check if this is a Bedrock player using the Geyser context helper
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
		DeviceOs:     convertDeviceOS(bedrockData.DeviceOS.ID),
		Language:     bedrockData.Language,
		UiProfile:    convertUIProfile(bedrockData.UIProfile),
		InputMode:    convertInputMode(bedrockData.InputMode),
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

// convertDeviceOS converts from Floodgate device OS ID to protobuf enum
func convertDeviceOS(deviceOSID int) pb.BedrockDeviceOS {
	switch deviceOSID {
	case 0:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_UNKNOWN
	case 1:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_ANDROID
	case 2:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_IOS
	case 3:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_MACOS
	case 4:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_AMAZON
	case 5:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_GEAR_VR
	case 6:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_HOLOLENS
	case 7:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_UWP
	case 8:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_X86
	case 9:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_DEDICATED
	case 10:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_APPLE_TV
	case 11:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_PLAYSTATION
	case 12:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_SWITCH
	case 13:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_XBOX
	case 14:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_PHONE
	case 15:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_LINUX
	default:
		return pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_UNKNOWN
	}
}

// convertUIProfile converts from Floodgate UI profile to protobuf enum
func convertUIProfile(uiProfile int) pb.BedrockUIProfile {
	switch uiProfile {
	case 0:
		return pb.BedrockUIProfile_BEDROCK_UI_PROFILE_CLASSIC
	case 1:
		return pb.BedrockUIProfile_BEDROCK_UI_PROFILE_POCKET
	default:
		return pb.BedrockUIProfile_BEDROCK_UI_PROFILE_UNSPECIFIED
	}
}

// convertInputMode converts from Floodgate input mode to protobuf enum
func convertInputMode(inputMode int) pb.BedrockInputMode {
	switch inputMode {
	case 0:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_UNKNOWN
	case 1:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_MOUSE
	case 2:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_TOUCH
	case 3:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_GAMEPAD
	case 4:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_MOTION_CONTROLLER
	default:
		return pb.BedrockInputMode_BEDROCK_INPUT_MODE_UNKNOWN
	}
}

// ConvertStrategyFromString converts from Go strategy string to protobuf enum
func ConvertStrategyFromString(strategy string) pb.LiteRouteStrategy {
	switch strategy {
	case "sequential":
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL
	case "random":
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM
	case "round-robin":
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN
	case "least-connections":
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS
	case "lowest-latency":
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY
	default:
		return pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL
	}
}

// ConvertStrategyToString converts from protobuf enum to Go strategy string
func ConvertStrategyToString(strategy pb.LiteRouteStrategy) string {
	switch strategy {
	case pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL:
		return "sequential"
	case pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM:
		return "random"
	case pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN:
		return "round-robin"
	case pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS:
		return "least-connections"
	case pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY:
		return "lowest-latency"
	default:
		return "sequential"
	}
}
