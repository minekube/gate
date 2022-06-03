package proto

import (
	"io"

	"github.com/go-logr/logr"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"go.minekube.com/gate/pkg/gate/proto"
)

type Encoder struct {
	dec       *packet.Encoder
	log       logr.Logger
	direction proto.Direction
}

func NewEncoder(w io.Writer, direction proto.Direction, log logr.Logger) *Encoder {
	return &Encoder{
		dec:       packet.NewEncoder(w),
		log:       log,
		direction: direction,
	}
}
