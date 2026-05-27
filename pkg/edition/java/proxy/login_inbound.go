package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

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
	delegate        *initialInbound
	sequenceCounter atomic.Int32

	// mu guards the fields below, which are accessed both from event-handler
	// goroutines (SendLoginPluginMessage) and the client read loop
	// (handleLoginPluginResponse). External callbacks (the message consumer,
	// onAllMessagesHandled) and connection I/O are always invoked without the
	// lock held to avoid re-entrant deadlocks.
	mu                   sync.Mutex
	outstandingResponses map[int]MessageConsumer
	loginMessagesToSend  deque.Deque[*packet.LoginPluginMessage]
	isLoginEventFired    bool
	onAllMessagesHandled func() error

	playerKey crypto.IdentifiedKey
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
	msg := &packet.LoginPluginMessage{
		ID:      id,
		Channel: identifier.ID(),
		Data:    contents,
	}

	l.mu.Lock()
	l.outstandingResponses[id] = consumer
	fired := l.isLoginEventFired
	if !fired {
		l.loginMessagesToSend.PushBack(msg)
	}
	l.mu.Unlock()

	if fired {
		return l.delegate.WritePacket(msg)
	}
	return nil
}

func (l *loginInboundConn) handleLoginPluginResponse(res *packet.LoginPluginResponse) (err error) {
	l.mu.Lock()
	consumer, ok := l.outstandingResponses[res.ID]
	if !ok {
		l.mu.Unlock()
		return nil
	}
	delete(l.outstandingResponses, res.ID)
	l.mu.Unlock()

	// Invoke the consumer without the lock held; it may call back into
	// SendLoginPluginMessage (which also takes the lock).
	if res.Success {
		err = consumer.OnMessageResponse(res.Data)
	} else {
		err = consumer.OnMessageResponse(nil)
	}

	// After the consumer ran (it may have queued more messages), fire the
	// all-handled callback if nothing is outstanding.
	l.mu.Lock()
	done := len(l.outstandingResponses) == 0
	onAllMessagesHandled := l.onAllMessagesHandled
	l.mu.Unlock()
	if done && onAllMessagesHandled != nil {
		err = errors.Join(err, onAllMessagesHandled())
	}
	return err
}

func (l *loginInboundConn) loginEventFired(onAllMessagesHandled func() error) error {
	l.mu.Lock()
	l.isLoginEventFired = true
	l.onAllMessagesHandled = onAllMessagesHandled
	msgs := make([]*packet.LoginPluginMessage, 0, l.loginMessagesToSend.Len())
	for l.loginMessagesToSend.Len() != 0 {
		msgs = append(msgs, l.loginMessagesToSend.PopFront())
	}
	l.mu.Unlock()

	if len(msgs) == 0 {
		return onAllMessagesHandled()
	}
	for _, msg := range msgs {
		if err := l.delegate.BufferPacket(msg); err != nil {
			return err
		}
	}
	return l.delegate.Flush()
}

// clearOnAllMessagesHandled removes the onAllMessagesHandled callback.
// Called before the Modern Forge login relay to prevent the PreLogin
// completion callback from re-firing when relay responses are processed.
func (l *loginInboundConn) clearOnAllMessagesHandled() {
	l.mu.Lock()
	l.onAllMessagesHandled = nil
	l.mu.Unlock()
}

func (l *loginInboundConn) disconnect(reason component.Component) error {
	defer l.cleanup()
	return l.delegate.disconnect(reason)
}

func (l *loginInboundConn) cleanup() {
	l.mu.Lock()
	l.loginMessagesToSend.Clear()
	l.outstandingResponses = map[int]MessageConsumer{}
	l.onAllMessagesHandled = nil
	l.mu.Unlock()
}
