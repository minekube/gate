package modinfo

var Default = &ModInfo{Type: "FML"}

type ModInfo struct {
	Type string `json:"type"`
	Mods []Mod  `json:"modList"`
}

type Mod struct {
	ID      string `json:"modid"`
	Version string `json:"version"`
}
