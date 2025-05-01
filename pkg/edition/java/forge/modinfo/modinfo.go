// Package modinfo provides mod information used in Forge ping responses.
package modinfo

import "errors"

var Default = &ModInfo{Type: "FML"}

// ModInfo represents a mod info.
type ModInfo struct {
	Type string `json:"type"`
	Mods []Mod  `json:"modList"`
}

// Mod represents a mod.
type Mod struct {
	ID      string `json:"modid"`
	Version string `json:"version"`
}

// Validate validates the mod.
func (m *Mod) Validate() error {
	if m == nil {
		return errors.New("mod info is nil")
	}
	if m.Version == "" {
		return errors.New("mod version is required")
	}
	if m.ID == "" {
		return errors.New("mod id is required")
	}
	if len(m.ID) > 128 {
		return errors.New("mod id is too long")
	}
	if len(m.Version) > 128 {
		return errors.New("mod version is too long")
	}
	return nil
}
