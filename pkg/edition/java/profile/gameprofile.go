package profile

import (
	"encoding/json"
	"fmt"

	"go.minekube.com/gate/pkg/util/uuid"
)

// GameProfile is a Mojang game profile.
type GameProfile struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Properties []Property `json:"properties"`
}

func (g *GameProfile) String() string {
	return fmt.Sprintf("GameProfile{ID:%s,Name:%s,Properties:%s}",
		g.ID, g.Name, g.Properties)
}

// NewOffline returns the new GameProfile for an offline profile.
func NewOffline(username string) *GameProfile {
	return &GameProfile{
		Name: username,
		ID:   uuid.OfflinePlayerUUID(username),
	}
}

func (g *GameProfile) MarshalJSON() ([]byte, error) {
	type Embed GameProfile
	return json.Marshal(&struct {
		ID string `json:"id"`
		*Embed
	}{
		ID:    g.ID.Undashed(),
		Embed: (*Embed)(g),
	})
}

func (g *GameProfile) UnmarshalJSON(data []byte) (err error) {
	type Embed GameProfile
	s := &struct {
		ID string `json:"id"`
		*Embed
	}{
		Embed: (*Embed)(g),
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}
	g.ID, err = uuid.Parse(s.ID)
	return
}

// Property is a Mojang profile property.
type Property struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature"`
}

func (p *Property) String() string {
	return fmt.Sprintf("Property{Name:%s,Value:%s,Signature:%s}",
		p.Name, p.Value, p.Signature)
}
