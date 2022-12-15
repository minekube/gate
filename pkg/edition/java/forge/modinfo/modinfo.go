// Package modinfo provides mod information used in Forge ping responses.
package modinfo

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
