package proxy

import (
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
	"go.uber.org/multierr"
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

type loginInboundConn struct {
	delegate             *initialInbound
	outstandingResponses map[int]MessageConsumer
	sequenceCounter      atomic.Int32
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

func (l *loginInboundConn) RemoteAddr() net.Addr { return l.delegate.RemoteAddr() }

func (l *loginInboundConn) Active() bool { return l.delegate.Active() }

func (l *loginInboundConn) Closed() <-chan struct{} { return l.delegate.Closed() }

func (l *loginInboundConn) IdentifiedKey() crypto.IdentifiedKey { return l.playerKey }

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
			err = multierr.Combine(err, l.onAllMessagesHandled())
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
	return l.delegate.flush()
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
