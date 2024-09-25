package packet

import (
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type CustomReportDetails struct {
	Details map[string]string
}

func (p *CustomReportDetails) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := protoutil.PanicWriter(wr)
	w.VarInt(len(p.Details))
	for key, value := range p.Details {
		w.String(key)
		w.String(value)
	}
	return nil
}

func (p *CustomReportDetails) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r := protoutil.PanicReader(rd)
	var detailsCount int
	r.VarInt(&detailsCount)
	p.Details = make(map[string]string, detailsCount)
	for i := 0; i < detailsCount; i++ {
		var key, value string
		r.String(&key)
		r.String(&value)
		p.Details[key] = value
	}
	return
}
