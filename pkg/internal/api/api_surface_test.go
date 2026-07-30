package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestConfigAPISurfaceIsNarrowAndVersioned(t *testing.T) {
	file := pb.File_minekube_gate_v1_gate_service_proto

	require.Nil(t, file.Services().ByName("GateLiteService"))
	for _, name := range []protoreflect.Name{
		"GateConfig",
		"JavaConfig",
		"ConnectConfig",
		"ForwardingConfig",
		"LiteConfig",
		"StatusConfig",
		"APIConfig",
	} {
		require.Nilf(t, file.Messages().ByName(name), "%s must not mirror the YAML config", name)
	}

	getConfig := file.Messages().ByName("GetConfigResponse")
	require.NotNil(t, getConfig.Fields().ByName("version"))

	applyConfig := file.Messages().ByName("ApplyConfigRequest")
	require.NotNil(t, applyConfig.Fields().ByName("config"))
	require.NotNil(t, applyConfig.Fields().ByName("merge_patch"))
	require.NotNil(t, applyConfig.Fields().ByName("if_match"))
	require.NotNil(t, applyConfig.Oneofs().ByName("input"))
}
