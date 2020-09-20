package codec

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
	"io"
)

type Decoder struct {
	dec       *packet.Decoder
	log       logr.Logger
	direction proto.Direction
}

func NewDecoder(r io.Reader, direction proto.Direction, log logr.Logger) *Decoder {
	return &Decoder{
		dec:       packet.NewDecoder(r),
		log:       log,
		direction: direction,
	}
}
