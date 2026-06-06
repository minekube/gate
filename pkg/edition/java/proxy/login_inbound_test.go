package proxy

import (
	"sync"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
)

type funcMessageConsumer func([]byte) error

func (f funcMessageConsumer) OnMessageResponse(b []byte) error { return f(b) }

// loginInboundConn is driven from two goroutines during a Forge login relay: the
// relay/backend goroutine calls SendLoginPluginMessage while the client read loop
// calls handleLoginPluginResponse. Its shared state must be safe for that.
// Run with -race to detect unsynchronized access.
func TestLoginInboundConn_ConcurrentSendAndResponse(t *testing.T) {
	l := newTestLoginInboundConn(&testMinecraftConn{})
	id, err := message.ChannelIdentifierFrom("test:channel")
	if err != nil {
		t.Fatal(err)
	}
	consumer := funcMessageConsumer(func([]byte) error { return nil })

	const n = 2000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_ = l.SendLoginPluginMessage(id, []byte{0x01}, consumer)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_ = l.handleLoginPluginResponse(&packet.LoginPluginResponse{ID: i, Success: true})
		}
	}()
	wg.Wait()
}
