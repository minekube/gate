package proxy

import "errors"

// Proxy is Gate's Bedrock edition of a Minecraft proxy.
type Proxy struct {
}

func (p *Proxy) Start(stop <-chan struct{}) error {
	return errors.New("unimplemented")
}
