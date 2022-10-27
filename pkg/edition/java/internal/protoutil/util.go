package protoutil

import (
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Protocol returns the protocol version of the given subject if provided.
func Protocol(subject any) (proto.Protocol, bool) {
	// this method is implemented by proxy player
	if p, ok := subject.(interface{ Protocol() proto.Protocol }); ok {
		return p.Protocol(), true
	}
	return version.Unknown.Protocol, false
}
