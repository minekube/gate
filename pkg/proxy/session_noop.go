package proxy

import (
	"go.minekube.com/gate/pkg/proto"
)

// A no-operation session handler can be wrapped to
// implement the sessionHandler interface.
type noOpSessionHandler struct{}

var _ sessionHandler = (*noOpSessionHandler)(nil)

func (noOpSessionHandler) handlePacket(proto.Packet)                {}
func (noOpSessionHandler) handleUnknownPacket(*proto.PacketContext) {}
func (noOpSessionHandler) disconnected()                            {}
func (noOpSessionHandler) deactivated()                             {}
func (noOpSessionHandler) activated()                               {}
