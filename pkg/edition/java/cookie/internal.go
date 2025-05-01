package cookie

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"go.minekube.com/common/minecraft/key"
	cpacket "go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/state"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type cookie struct {
	mu      sync.RWMutex
	key     key.Key
	payload []byte
}

func (c *cookie) Store(p proxy.Player) error {
	if strings.TrimSpace(c.key.String()) == "" {
		return errors.New("empty key")
	}

	if len(c.payload) > 5*1024 {
		return errors.New("payload size exceeds 5 kiB")
	}

	if p.Protocol().Lower(version.Minecraft_1_20_5) {
		return fmt.Errorf("%w: but player is on %s", proxy.ErrTransferUnsupportedClientProtocol, p.Protocol())
	}

	if p.State() != state.Play && p.State() != state.Config {
		return errors.New("CookieStore packet can only be send in the Play and Configuration State")
	}

	return p.WritePacket(&cpacket.CookieStore{
		Key:     c.key,
		Payload: c.payload,
	})
}

func request(p proxy.Player, key key.Key) error {
	if strings.TrimSpace(key.String()) == "" {
		return errors.New("empty key")
	}

	if p.Protocol().Lower(version.Minecraft_1_20_5) {
		return fmt.Errorf("%w: but player is on %s", proxy.ErrTransferUnsupportedClientProtocol, p.Protocol())
	}

	return p.WritePacket(&cpacket.CookieRequest{
		Key: key,
	})
}

func requestWithResult(p proxy.Player, key key.Key, ctx context.Context) ([]byte, error) {
	if strings.TrimSpace(key.String()) == "" {
		return nil, errors.New("empty key")
	}

	if p.Protocol().Lower(version.Minecraft_1_20_5) {
		return nil, fmt.Errorf("%w: but player is on %s", proxy.ErrTransferUnsupportedClientProtocol, p.Protocol())
	}

	responseChan := make(chan []byte, 1)
	errorChan := make(chan error, 1)
	defer close(responseChan)
	defer close(errorChan)

	r := proxy.RequestListenerPerPlayer[p]
	r.Mu.Lock()
	r.Pending[key.String()] = responseChan
	defer delete(r.Pending, key.String())
	r.Mu.Unlock()

	err := p.WritePacket(&cpacket.CookieRequest{
		Key: key,
	})

	if err != nil {
		errorChan <- err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errorChan:
		return nil, err
	case response := <-responseChan:
		return response, nil
	case <-p.Context().Done():
		return nil, errors.New("player disconnected")
	}
}

func (c *cookie) Key() key.Key {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.key
}

func (c *cookie) SetKey(key key.Key) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key == key {
		return
	}

	c.key = key
}

func (c *cookie) Payload() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.payload
}

func (c *cookie) SetPayload(payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if bytes.Equal(c.payload, payload) {
		return
	}

	c.payload = payload
}
