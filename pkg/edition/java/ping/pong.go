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
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/uuid"
	"gopkg.in/yaml.v3"
)

// ServerPing is a 1.7 and above server list ping response.
type ServerPing struct {
	Version     Version          `json:"version,omitempty" yaml:"version,omitempty"`
	Players     *Players         `json:"players,omitempty" yaml:"players,omitempty"`
	Description *component.Text  `json:"description" yaml:"description"`
	Favicon     favicon.Favicon  `json:"favicon,omitempty" yaml:"favicon,omitempty"`
	ModInfo     *modinfo.ModInfo `json:"modinfo,omitempty" yaml:"modinfo,omitempty"`
	ForgeData   *ForgeData       `json:"forgeData,omitempty" yaml:"forgeData,omitempty"` // Modern Forge (1.13+)
}

// ForgeData is the modern Forge (1.13+) ping response data.
// Used instead of ModInfo for FML2/FML3 clients.
type ForgeData struct {
	Channels          []ForgeChannel `json:"channels,omitempty"`
	Mods              []ForgeMod     `json:"mods,omitempty"`
	FMLNetworkVersion int            `json:"fmlNetworkVersion"` // 2 for FML2, 3 for FML3
	Truncated         bool           `json:"truncated,omitempty"`
}

// ForgeChannel represents a Forge network channel.
type ForgeChannel struct {
	Res      string `json:"res"`      // Resource location (e.g., "fml:handshake")
	Version  string `json:"version"`  // Channel version
	Required bool   `json:"required"` // Whether the channel is required
}

// ForgeMod represents a mod in the Forge ping response.
type ForgeMod struct {
	ModId     string `json:"modId"`     // Mod identifier
	ModMarker string `json:"modmarker"` // Mod version marker
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

	// Handle null or missing description (e.g., from backend server ping passthrough)
	if len(out.Description) == 0 || string(out.Description) == "null" {
		out.Alias.Description = &component.Text{} // empty component
	} else {
		var err error
		out.Alias.Description, err = componentutil.ParseTextComponent(out.Version.Protocol, string(out.Description))
		if err != nil {
			return fmt.Errorf("error decoding description: %w", err)
		}
	}

	*p = ServerPing(out.Alias)
	return nil
}

func (p *ServerPing) UnmarshalYAML(value *yaml.Node) error {
	type Alias ServerPing
	out := &struct {
		Description string `yaml:"description"` // override description type
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := value.Decode(out); err != nil {
		return fmt.Errorf("error decoding yaml: %w", err)
	}

	var err error
	p.Description, err = componentutil.ParseTextComponent(out.Version.Protocol, out.Description)
	if err != nil {
		return fmt.Errorf("error decoding description: %w", err)
	}

	*p = ServerPing(*out.Alias)
	return nil
}

func (p *ServerPing) MarshalYAML() (any, error) {
	b := new(strings.Builder)
	err := (&legacy.Legacy{}).Marshal(b, p.Description)
	if err != nil {
		return nil, fmt.Errorf("error encoding description: %w", err)
	}

	type Alias ServerPing
	return &struct {
		Description string
		*Alias
	}{
		Description: b.String(),
		Alias:       (*Alias)(p),
	}, nil
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
