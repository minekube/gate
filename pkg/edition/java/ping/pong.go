package ping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	protoversion "go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/uuid"
	"gopkg.in/yaml.v3"
)

// ServerPing is a 1.7 and above server list ping response.
type ServerPing struct {
	Version     Version             `json:"version,omitempty" yaml:"version,omitempty"`
	Players     *Players            `json:"players,omitempty" yaml:"players,omitempty"`
	Description component.Component `json:"description" yaml:"description"`
	Favicon     favicon.Favicon     `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	ModInfo     *modinfo.ModInfo    `json:"modinfo,omitempty" yaml:"modinfo,omitempty"`
	// Extra keeps the status members this struct does not model, verbatim, so a
	// backend's own advertisement survives Gate re-encoding the response.
	//
	// Modded servers keep their mod metadata there: Forge 1.13+ (FML2/FML3)
	// sends "forgeData" (mod list, channels, fmlNetworkVersion) and NeoForge
	// sends "neoforgeData". Neither is modeled above, so a client reading the
	// status through Gate saw a provider with no mod block at all - no mod
	// count, no channels, no FML network version. Carrying unmodeled members
	// through untouched keeps the proxy's status faithful to the backend it
	// stands for. Gate never interprets these values, it only re-emits them.
	Extra map[string]json.RawMessage `json:"-" yaml:"-"`
}

// Make sure ServerPing implements the interfaces at compile time.
var (
	_ json.Marshaler   = (*ServerPing)(nil)
	_ json.Unmarshaler = (*ServerPing)(nil)

	_ yaml.Marshaler   = (*ServerPing)(nil)
	_ yaml.Unmarshaler = (*ServerPing)(nil)
)

func (p *ServerPing) MarshalJSON() ([]byte, error) {
	b := new(bytes.Buffer)
	err := util.JsonCodec(p.Version.Protocol).Marshal(b, p.Description)
	if err != nil {
		return nil, err
	}

	type Alias ServerPing
	known, err := json.Marshal(&struct {
		Description json.RawMessage `json:"description"`
		*Alias
	}{
		Description: b.Bytes(),
		Alias:       (*Alias)(p),
	})
	if err != nil || len(p.Extra) == 0 {
		return known, err
	}

	// Merge the members Gate did not model back into the response. Modeled
	// fields win, so a stale key in Extra can never shadow the real value.
	merged := make(map[string]json.RawMessage, len(p.Extra)+5)
	if err = json.Unmarshal(known, &merged); err != nil {
		return nil, fmt.Errorf("error decoding marshaled ping: %w", err)
	}
	for k, v := range p.Extra {
		if _, modeled := merged[k]; !modeled && len(v) != 0 {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
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

	// Handle null or missing description (e.g., from backend server ping passthrough)
	if len(out.Description) == 0 || string(out.Description) == "null" {
		out.Alias.Description = &component.Text{} // empty component
	} else {
		var err error
		out.Alias.Description, err = componentutil.ParseComponent(out.Version.Protocol, string(out.Description))
		if err != nil {
			return fmt.Errorf("error decoding description: %w", err)
		}
	}

	out.Alias.Extra = unmodeledMembers(data)

	*p = ServerPing(out.Alias)
	return nil
}

// modeledStatusMembers are the top-level status members ServerPing knows about.
// Everything else is kept in ServerPing.Extra so it survives a re-encode.
var modeledStatusMembers = map[string]struct{}{
	"version":     {},
	"players":     {},
	"description": {},
	"favicon":     {},
	"modinfo":     {},
}

// unmodeledMembers returns the top-level members of a status JSON object that
// ServerPing does not model, or nil if there are none.
func unmodeledMembers(data []byte) map[string]json.RawMessage {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	var extra map[string]json.RawMessage
	for k, v := range all {
		// encoding/json matches struct fields case-insensitively, so compare
		// lowercased to keep a differently-cased modeled key out of Extra.
		if _, modeled := modeledStatusMembers[strings.ToLower(k)]; modeled {
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[k] = v
	}
	return extra
}

func (p *ServerPing) UnmarshalYAML(value *yaml.Node) error {
	type Alias ServerPing
	out := &struct {
		Description string `yaml:"description"` // override description type
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := configutil.DecodeYAMLStrict(value, out); err != nil {
		return fmt.Errorf("error decoding yaml: %w", err)
	}

	var err error
	// YAML status config cannot express the raw passthrough members, so a
	// decoded config must not inherit stale ones from a reused value.
	out.Alias.Extra = nil
	p.Description, err = componentutil.ParseComponent(out.Version.Protocol, out.Description)
	if err != nil {
		return fmt.Errorf("error decoding description: %w", err)
	}

	*p = ServerPing(*out.Alias)
	return nil
}

func (p *ServerPing) MarshalYAML() (any, error) {
	description := ""
	if text, ok := p.Description.(*component.Text); ok {
		b := new(strings.Builder)
		if err := (&legacy.Legacy{}).Marshal(b, text); err == nil {
			description = b.String()
		} else {
			description, err = p.marshalDescriptionJSON()
			if err != nil {
				return nil, err
			}
		}
	} else if p.Description != nil {
		var err error
		description, err = p.marshalDescriptionJSON()
		if err != nil {
			return nil, err
		}
	}

	type Alias ServerPing
	return &struct {
		Description string
		*Alias
	}{
		Description: description,
		Alias:       (*Alias)(p),
	}, nil
}

func (p *ServerPing) marshalDescriptionJSON() (string, error) {
	protocol := p.Version.Protocol
	if protocol == 0 {
		protocol = protoversion.MaximumVersion.Protocol
	}
	b, err := util.Marshal(protocol, p.Description)
	if err != nil {
		return "", fmt.Errorf("error encoding description: %w", err)
	}
	return string(b), nil
}

type Version struct {
	Protocol proto.Protocol `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Name     string         `json:"name,omitempty" yaml:"name,omitempty"`
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
