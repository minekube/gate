package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func TestBedrockEnumConversions(t *testing.T) {
	require.Equal(t, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_ANDROID, convertDeviceOS(1))
	require.Equal(t, pb.BedrockDeviceOS_BEDROCK_DEVICE_OS_UNKNOWN, convertDeviceOS(999))
	require.Equal(t, pb.BedrockUIProfile_BEDROCK_UI_PROFILE_POCKET, convertUIProfile(1))
	require.Equal(t, pb.BedrockUIProfile_BEDROCK_UI_PROFILE_UNSPECIFIED, convertUIProfile(999))
	require.Equal(t, pb.BedrockInputMode_BEDROCK_INPUT_MODE_GAMEPAD, convertInputMode(3))
	require.Equal(t, pb.BedrockInputMode_BEDROCK_INPUT_MODE_UNKNOWN, convertInputMode(999))
}
