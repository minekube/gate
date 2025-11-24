package api

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func TestConvertStrategyFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected pb.LiteRouteStrategy
	}{
		{
			name:     "sequential",
			input:    string(config.StrategySequential),
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
		},
		{
			name:     "random",
			input:    string(config.StrategyRandom),
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
		},
		{
			name:     "round_robin",
			input:    string(config.StrategyRoundRobin),
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
		},
		{
			name:     "least_connections",
			input:    string(config.StrategyLeastConnections),
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS,
		},
		{
			name:     "lowest_latency",
			input:    string(config.StrategyLowestLatency),
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY,
		},
		{
			name:     "unknown strategy defaults to sequential",
			input:    "unknown",
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
		},
		{
			name:     "empty string defaults to sequential",
			input:    "",
			expected: pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertStrategyFromString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertStrategyToString(t *testing.T) {
	tests := []struct {
		name     string
		input    pb.LiteRouteStrategy
		expected string
	}{
		{
			name:     "sequential",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
			expected: string(config.StrategySequential),
		},
		{
			name:     "random",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
			expected: string(config.StrategyRandom),
		},
		{
			name:     "round_robin",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
			expected: string(config.StrategyRoundRobin),
		},
		{
			name:     "least_connections",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS,
			expected: string(config.StrategyLeastConnections),
		},
		{
			name:     "lowest_latency",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY,
			expected: string(config.StrategyLowestLatency),
		},
		{
			name:     "unspecified defaults to sequential",
			input:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_UNSPECIFIED,
			expected: string(config.StrategySequential),
		},
		{
			name:     "invalid enum value defaults to sequential",
			input:    pb.LiteRouteStrategy(999),
			expected: string(config.StrategySequential),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertStrategyToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStrategyConversionRoundTrip(t *testing.T) {
	// Test that converting to string and back yields the same result
	strategies := []pb.LiteRouteStrategy{
		pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
		pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
		pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
		pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS,
		pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY,
	}

	for _, strategy := range strategies {
		t.Run(strategy.String(), func(t *testing.T) {
			str := ConvertStrategyToString(strategy)
			back := ConvertStrategyFromString(str)
			assert.Equal(t, strategy, back)
		})
	}
}

func TestInternalStrategyConversion(t *testing.T) {
	// Test conversion between internal config strategies and protobuf strategies
	tests := []struct {
		name     string
		internal config.Strategy
		proto    pb.LiteRouteStrategy
	}{
		{
			name:     "sequential",
			internal: config.StrategySequential,
			proto:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_SEQUENTIAL,
		},
		{
			name:     "random",
			internal: config.StrategyRandom,
			proto:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_RANDOM,
		},
		{
			name:     "round_robin",
			internal: config.StrategyRoundRobin,
			proto:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_ROUND_ROBIN,
		},
		{
			name:     "least_connections",
			internal: config.StrategyLeastConnections,
			proto:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LEAST_CONNECTIONS,
		},
		{
			name:     "lowest_latency",
			internal: config.StrategyLowestLatency,
			proto:    pb.LiteRouteStrategy_LITE_ROUTE_STRATEGY_LOWEST_LATENCY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert internal -> proto
			protoResult := ConvertStrategyFromString(string(tt.internal))
			assert.Equal(t, tt.proto, protoResult)

			// Convert proto -> internal
			internalResult := config.Strategy(ConvertStrategyToString(tt.proto))
			assert.Equal(t, tt.internal, internalResult)
		})
	}
}

// Note: Player and Server conversion tests would require setting up actual proxy instances
// which is complex for unit tests. These conversion functions are simple enough that
// testing the strategy conversions below provides good coverage.
// Integration tests would cover the full player/server conversion flows.

func TestConvertDeviceOS(t *testing.T) {
	tests := []struct {
		input    int
		expected pb.BedrockDeviceOS
	}{
		{0, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_UNKNOWN},
		{1, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_ANDROID},
		{2, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_IOS},
		{3, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_MACOS},
		{4, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_AMAZON},
		{5, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_GEAR_VR},
		{6, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_HOLOLENS},
		{7, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_UWP},
		{8, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_X86},
		{9, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_DEDICATED},
		{10, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_APPLE_TV},
		{11, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_PLAYSTATION},
		{12, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_SWITCH},
		{13, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_XBOX},
		{14, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_WINDOWS_PHONE},
		{15, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_LINUX},
		{999, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_UNKNOWN}, // Unknown ID
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			result := convertDeviceOS(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertUIProfile(t *testing.T) {
	tests := []struct {
		input    int
		expected pb.BedrockUIProfile
	}{
		{0, pb.BedrockUIProfile_BEDROCK_UI_PROFILE_CLASSIC},
		{1, pb.BedrockUIProfile_BEDROCK_UI_PROFILE_POCKET},
		{999, pb.BedrockUIProfile_BEDROCK_UI_PROFILE_UNSPECIFIED}, // Unknown ID
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			result := convertUIProfile(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertInputMode(t *testing.T) {
	tests := []struct {
		input    int
		expected pb.BedrockInputMode
	}{
		{0, pb.BedrockInputMode_BEDROCK_INPUT_MODE_UNKNOWN},
		{1, pb.BedrockInputMode_BEDROCK_INPUT_MODE_MOUSE},
		{2, pb.BedrockInputMode_BEDROCK_INPUT_MODE_TOUCH},
		{3, pb.BedrockInputMode_BEDROCK_INPUT_MODE_GAMEPAD},
		{4, pb.BedrockInputMode_BEDROCK_INPUT_MODE_MOTION_CONTROLLER},
		{999, pb.BedrockInputMode_BEDROCK_INPUT_MODE_UNKNOWN}, // Unknown ID
	}

	for _, tt := range tests {
		t.Run(tt.expected.String(), func(t *testing.T) {
			result := convertInputMode(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}