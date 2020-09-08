package modinfo

var Default = &ModInfo{Type: "FML"}

type ModInfo struct {
	Type string
	Mods []Mod
}

type Mod struct {
	ID      string `json:"modid"`
	Version string
}
