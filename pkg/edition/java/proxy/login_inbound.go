package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/gammazero/deque"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.uber.org/atomic"
)

type (
	// LoginPhaseConnection allows the server to communicate with a
	// client logging into the proxy using login plugin messages.
	LoginPhaseConnection interface {
		Inbound
		crypto.KeyIdentifiable
		SendLoginPluginMessage(identifier message.ChannelIdentifier, contents []byte, consumer MessageConsumer) error
	}

	MessageConsumer interface {
		OnMessageResponse(responseBody []byte) error // responseBody may be empty
	}
)

// Inbound is an incoming connection to the proxy.
type Inbound interface {
	Protocol() proto.Protocol                // The current protocol version the connection uses.
	VirtualHost() net.Addr                   // The hostname, the client sent us, to join the server, if applicable.
	HandshakeIntent() packet.HandshakeIntent // The intent of the handshake.
	RemoteAddr() net.Addr                    // The player's IP address.
	Active() bool                            // Whether the connection remains active.
	// Context returns the connection's context that can be used to know when the connection was closed.
	// (e.g. for canceling work in an event handler)
	Context() context.Context
}

type loginInboundConn struct {
	delegate             *initialInbound
	outstandingResponses map[int]MessageConsumer
	sequenceCounter      atomic.Int32
	loginMessagesToSend  deque.Deque[*packet.LoginPluginMessage]
	isLoginEventFired    bool
	onAllMessagesHandled func() error

	playerKey crypto.IdentifiedKey

	// forgeResponses maps message IDs to channels for direct Forge login plugin forwarding
	forgeResponses map[int]chan *packet.LoginPluginResponse
}

func newLoginInboundConn(delegate *initialInbound) *loginInboundConn {
	return &loginInboundConn{
		delegate:             delegate,
		outstandingResponses: map[int]MessageConsumer{},
	}
}

var _ LoginPhaseConnection = (*loginInboundConn)(nil)

func (l *loginInboundConn) Protocol() proto.Protocol { return l.delegate.Protocol() }

func (l *loginInboundConn) VirtualHost() net.Addr { return l.delegate.VirtualHost() }

func (l *loginInboundConn) HandshakeIntent() packet.HandshakeIntent {
	return l.delegate.HandshakeIntent()
}

func (l *loginInboundConn) RemoteAddr() net.Addr { return l.delegate.RemoteAddr() }

func (l *loginInboundConn) Active() bool { return l.delegate.Active() }

func (l *loginInboundConn) IdentifiedKey() crypto.IdentifiedKey { return l.playerKey }

func (l *loginInboundConn) Context() context.Context { return l.delegate.Context() }

func (l *loginInboundConn) SendLoginPluginMessage(identifier message.ChannelIdentifier, contents []byte, consumer MessageConsumer) error {
	if identifier == nil {
		return errors.New("missing identifier")
	}
	if len(contents) == 0 {
		return errors.New("missing contents")
	}
	if consumer == nil {
		return errors.New("missing consumer")
	}
	if l.delegate.Protocol() < version.Minecraft_1_13.Protocol {
		return fmt.Errorf("login plugin messages can only be send to clients running Minecraft %s and above, but is %s",
			version.Minecraft_1_13, l.delegate.Protocol())
	}

	id := int(l.sequenceCounter.Inc())
	l.outstandingResponses[id] = consumer

	msg := &packet.LoginPluginMessage{
		ID:      id,
		Channel: identifier.ID(),
		Data:    contents,
	}
	if l.isLoginEventFired {
		return l.delegate.WritePacket(msg)
	}
	l.loginMessagesToSend.PushBack(msg)
	return nil
}

func (l *loginInboundConn) handleLoginPluginResponse(res *packet.LoginPluginResponse) (err error) {
	consumer, ok := l.outstandingResponses[res.ID]
	if !ok {
		return nil
	}
	delete(l.outstandingResponses, res.ID)

	defer func() {
		if len(l.outstandingResponses) == 0 && l.onAllMessagesHandled != nil {
			err = errors.Join(err, l.onAllMessagesHandled())
		}
	}()
	if res.Success {
		return consumer.OnMessageResponse(res.Data)
	}
	return consumer.OnMessageResponse(nil)
}

func (l *loginInboundConn) loginEventFired(onAllMessagesHandled func() error) error {
	l.isLoginEventFired = true
	l.onAllMessagesHandled = onAllMessagesHandled
	if l.loginMessagesToSend.Len() == 0 {
		return onAllMessagesHandled()
	}
	for l.loginMessagesToSend.Len() != 0 {
		msg := l.loginMessagesToSend.PopFront()
		if err := l.delegate.BufferPacket(msg); err != nil {
			return err
		}
	}
	return l.delegate.Flush()
}

func (l *loginInboundConn) disconnect(reason component.Component) error {
	defer l.cleanup()
	return l.delegate.disconnect(reason)
}

func (l *loginInboundConn) cleanup() {
	l.loginMessagesToSend.Clear()
	l.outstandingResponses = map[int]MessageConsumer{}
	l.onAllMessagesHandled = nil
}

// enableImmediateSend sets the login phase to immediately send messages
// instead of queuing them. This is used for Modern Forge login plugin
// message forwarding where we need to forward messages from the backend
// to the client immediately without waiting for the login event flow.
func (l *loginInboundConn) enableImmediateSend() {
	l.isLoginEventFired = true
}

// registerForgeResponse registers a channel to receive a LoginPluginResponse with the given ID.
func (l *loginInboundConn) registerForgeResponse(id int, ch chan *packet.LoginPluginResponse) {
	if l.forgeResponses == nil {
		l.forgeResponses = make(map[int]chan *packet.LoginPluginResponse)
	}
	l.forgeResponses[id] = ch
}

// unregisterForgeResponse removes a registered Forge response channel.
func (l *loginInboundConn) unregisterForgeResponse(id int) {
	if l.forgeResponses != nil {
		delete(l.forgeResponses, id)
	}
}

// handleForgeLoginPluginResponse handles a LoginPluginResponse for Forge forwarding.
// Returns true if the response was handled as a Forge response.
func (l *loginInboundConn) handleForgeLoginPluginResponse(res *packet.LoginPluginResponse) bool {
	if l.forgeResponses == nil {
		return false
	}
	ch, ok := l.forgeResponses[res.ID]
	if !ok {
		return false
	}
	select {
	case ch <- res:
	default:
		// Channel full, drop
	}
	return true
}
