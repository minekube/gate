package modinfo

var Default = &ModInfo{Type: "FML"}

type ModInfo struct {
	Type string
	Mods []Mod
}

type Mod struct {
	Id      string `json:"modid"`
	Version string
}
