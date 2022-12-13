package tablist

import (
	"fmt"

	"go.minekube.com/common/minecraft/component"
)

type LegacyEntry struct {
	KeyedEntry
}

func (e *LegacyEntry) SetDisplayName(name component.Component) error {
	// We have to remove first if updating
	err := e.Entry.OwningTabList.RemoveAll(e.Profile().ID)
	if err != nil {
		return fmt.Errorf("error removing legacy entry %s before SetDisplayName: %w",
			e.Profile(), err)
	}
	return e.Entry.SetDisplayName(name)
}
