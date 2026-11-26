package proxy

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
)

// forgeBackendStatus is a Forge 1.20.1 (FML3) server list response: the
// "forgeData" block is what the client reads to classify the connection as
// modded. On 1.20.1 the mod inventory is in the compressed "d" member.
const forgeBackendStatus = `{
  "description": {"text": "§b❄ Chillzone §b❄ §fModpack [1.0.1]"},
  "favicon": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==",
  "forgeData": {
    "channels": [],
    "d": "AAF8AAADfAAABAAAAA==",
    "fmlNetworkVersion": 3,
    "mods": [],
    "truncated": false
  },
  "players": {"max": 20, "online": 0},
  "version": {"name": "1.20.1", "protocol": 763}
}`

type statusCaptureConn struct {
	netmc.MinecraftConn
	written []proto.Packet
}

func (c *statusCaptureConn) WritePacket(p proto.Packet) error {
	c.written = append(c.written, p)
	return nil
}

func (c *statusCaptureConn) Close() error { return nil }

type activeStatusInbound struct{ Inbound }

func (*activeStatusInbound) Active() bool { return true }

// TestStatusReencodeKeepsBackendForgeData reproduces the Connect edge status
// path: the backend's response is resolved by an out-of-process resolver
// (moxy pings the endpoint through the tunnel), unmarshalled into a
// ping.ServerPing so a PingEvent subscriber can adjust it, and marshalled again
// before it is written to the client. The backend's forgeData must survive
// that re-encode, or the Forge client classifies the proxy as vanilla.
func TestStatusReencodeKeepsBackendForgeData(t *testing.T) {
	conn := &statusCaptureConn{}
	eventMgr := event.New()
	// Stand-in for moxy's PingEvent handler: it replaces the event ping with
	// its own value, exactly like the real resolver does.
	var seen *PingEvent
	event.Subscribe(eventMgr, 0, func(e *PingEvent) {
		seen = e
		pong := e.Ping()
		pong.Players.Online = 4
		e.SetPing(pong)
	})

	h := &statusSessionHandler{
		sessionHandlerDeps: &sessionHandlerDeps{eventMgr: eventMgr},
		conn:               conn,
		inbound:            &activeStatusInbound{},
		log:                logr.Discard(),
		resolvePingResponse: func(log logr.Logger, _ *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error) {
			return log, &packet.StatusResponse{Status: forgeBackendStatus}, nil
		},
	}

	h.handleStatusRequest(&proto.PacketContext{Packet: &packet.StatusRequest{}})
	require.NotNil(t, seen, "the ping event must reach subscribers")
	require.Len(t, conn.written, 1)

	res, ok := conn.written[0].(*packet.StatusResponse)
	require.Truef(t, ok, "expected a StatusResponse, got %T", conn.written[0])

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(res.Status), &doc))
	require.Contains(t, doc, "forgeData", "the client would classify this proxy as vanilla")
	require.JSONEq(t, `{"channels":[],"d":"AAF8AAADfAAABAAAAA==","fmlNetworkVersion":3,"mods":[],"truncated":false}`,
		string(doc["forgeData"]))
	// The subscriber's edit still applied, so the passthrough does not freeze
	// the response at the backend's values.
	require.JSONEq(t, `{"max":20,"online":4}`, string(doc["players"]))
}

// TestStatusReencodeKeepsVanillaStatusVanilla is the counterpart guard: a
// vanilla backend response must not gain a modded block on the same path.
func TestStatusReencodeKeepsVanillaStatusVanilla(t *testing.T) {
	conn := &statusCaptureConn{}
	eventMgr := event.New()
	event.Subscribe(eventMgr, 0, func(e *PingEvent) {})
	const vanilla = `{"description":{"text":"Vanilla"},"players":{"max":20,"online":0},"version":{"name":"1.20.1","protocol":763}}`

	h := &statusSessionHandler{
		sessionHandlerDeps: &sessionHandlerDeps{eventMgr: eventMgr},
		conn:               conn,
		inbound:            &activeStatusInbound{},
		log:                logr.Discard(),
		resolvePingResponse: func(log logr.Logger, _ *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error) {
			return log, &packet.StatusResponse{Status: vanilla}, nil
		},
	}

	h.handleStatusRequest(&proto.PacketContext{Packet: &packet.StatusRequest{}})
	require.Len(t, conn.written, 1)
	res := conn.written[0].(*packet.StatusResponse)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(res.Status), &doc))
	require.NotContains(t, doc, "forgeData")
	require.NotContains(t, doc, "modinfo")
}
