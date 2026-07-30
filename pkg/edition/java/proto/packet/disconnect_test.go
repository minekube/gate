package packet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

func TestNewDisconnectNilReasonEncodes(t *testing.T) {
	p := NewDisconnect(nil, version.Minecraft_1_20_2.Protocol, states.PlayState)
	require.NotNil(t, p.Reason)

	var buf bytes.Buffer
	err := p.Encode(&proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20_2.Protocol,
	}, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, buf.Bytes())
}

func TestNewDisconnectTypedNilReasonEncodes(t *testing.T) {
	var reason *component.Text
	p := NewDisconnect(reason, version.Minecraft_1_20_2.Protocol, states.PlayState)
	require.NotNil(t, p.Reason)

	var buf bytes.Buffer
	err := p.Encode(&proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20_2.Protocol,
	}, &buf)
	require.NoError(t, err)
	require.NotEmpty(t, buf.Bytes())
}
