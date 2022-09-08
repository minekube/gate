package ping

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
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
	err := util.JsonCodec(p.Version.Protocol).Marshal(b, p.Description)
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
func (p *ServerPing) UnmarshalJSON(data []byte) error {
	type Alias ServerPing
	out := &struct {
		Alias
		Description json.RawMessage `json:"description"` // override description type
	}{}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("error decoding json: %w", err)
	}

	if string(out.Description) != "{}" {
		description, err := util.JsonCodec(out.Version.Protocol).Unmarshal(out.Description)
		if err != nil {
			return fmt.Errorf("error decoding description: %w", err)
		}

		var ok bool
		out.Alias.Description, ok = description.(*component.Text)
		if !ok {
			return fmt.Errorf("unmmarshalled description is not a TextComponent, but %T", description)
		}
	}

	*p = ServerPing(out.Alias)
	return nil
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
