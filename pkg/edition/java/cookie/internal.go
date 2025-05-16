package cookie

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/internal/methods"
	pkt "go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func validate(c *Cookie, cli Client) error {
	if err := validateCookie(c); err != nil {
		return err
	}
	return isProtocolSupported(cli, true)
}

func validateKey(key key.Key) error {
	if key == nil {
		return errors.New("key is nil")
	}
	if strings.TrimSpace(key.String()) == "" {
		return errors.New("empty key")
	}
	return nil
}

func validateCookie(c *Cookie) error {
	if c == nil {
		return errors.New("cookie is nil")
	}
	if err := validateKey(c.Key); err != nil {
		return err
	}
	if len(c.Payload) > MaxPayloadSize {
		return errors.New("cookie payload size exceeds 5 kiB")
	}
	return nil
}

func isProtocolSupported(c Client, checkState bool) error {
	p, ok := methods.Protocol(c)
	if !ok {
		return fmt.Errorf("%w: %T does not implement Client", ErrUnsupportedClientProtocol, c)
	}
	if p.Lower(version.Minecraft_1_20_5) {
		return fmt.Errorf("%w: client is on %s", ErrUnsupportedClientProtocol, p)
	}
	if checkState {
		s, ok := methods.State(c)
		if !ok {
			return fmt.Errorf("%w: %T does not implement Client", ErrUnsupportedState, c)
		}
		if s != states.ConfigState && s != states.PlayState {
			return fmt.Errorf("%w: client is on %s", ErrUnsupportedState, s)
		}
	}
	return nil
}

func store(cli Client, c *Cookie) error {
	if err := validate(c, cli); err != nil {
		return err
	}

	// TODO fire cookie store event

	return cli.WritePacket(&pkt.CookieStore{
		Key:     c.Key,
		Payload: c.Payload,
	})
}

func request(ctx context.Context, cli Client, key key.Key, eventMgr event.Manager) (*Cookie, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if err := isProtocolSupported(cli, false); err != nil {
		return nil, err
	}

	// TODO fire cookie request event

	responseChan := make(chan *Cookie, 1)
	defer close(responseChan)

	var once sync.Once
	unsub := event.Subscribe(eventMgr, 0, func(e *proxy.CookieReceiveEvent) {
		if e.Key().String() == key.String() {
			once.Do(func() {
				responseChan <- &Cookie{
					Key:     e.Key(),
					Payload: e.Payload(),
				}
			})
		}
	})
	defer unsub()

	err := cli.WritePacket(&pkt.CookieRequest{
		Key: key,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultRequestTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-cli.Context().Done():
		return nil, fmt.Errorf("player disconnected: %w", cli.Context().Err())
	case c := <-responseChan:
		return c, nil
	}
}

func requestAndForget(cli Client, key key.Key) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := isProtocolSupported(cli, false); err != nil {
		return err
	}

	// TODO fire cookie request event

	return cli.WritePacket(&pkt.CookieRequest{
		Key: key,
	})
}
