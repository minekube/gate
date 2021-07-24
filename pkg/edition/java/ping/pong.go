package ping

import (
	"bytes"
	"encoding/json"
	"go.minekube.com/common/minecraft/component"
	util2 "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/uuid"
)

// ServerPing is a 1.7 and above server list ping response.
type ServerPing struct {
	Version     Version         `json:"version"`
	Players     *Players        `json:"players"`
	Description *component.Text `json:"description"`
	Favicon     favicon.Favicon `json:"favicon,omitempty"`
}

func (p *ServerPing) MarshalJSON() ([]byte, error) {
	b := new(bytes.Buffer)
	err := util2.JsonCodec(p.Version.Protocol).Marshal(b, p.Description)
	if err != nil {
		return nil, err
	}

	type Alias ServerPing
	return json.Marshal(&struct {
		Description json.RawMessage `json:"description"`
		*Alias
	}{
		Description: b.Bytes(),
		Alias:       (*Alias)(p),
	})
}

type Version struct {
	Protocol proto.Protocol `json:"protocol"`
	Name     string         `json:"name"`
}

type Players struct {
	Online int            `json:"online"`
	Max    int            `json:"max"`
	Sample []SamplePlayer `json:"sample,omitempty"`
}

type SamplePlayer struct {
	Name string    `json:"name"`
	ID   uuid.UUID `json:"id"`
}
