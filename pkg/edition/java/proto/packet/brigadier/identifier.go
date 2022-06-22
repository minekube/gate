package brigadier

import (
	"errors"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type (
	argIdentifier struct {
		id          string
		versionByID map[proto.Protocol]int
	}
	versionSet struct {
		version *proto.Version
		id      int
	}
)

func newArgIdentifier(id string, versions ...versionSet) (argIdentifier, error) {
	identifier := argIdentifier{
		id:          id,
		versionByID: map[proto.Protocol]int{},
	}
	if len(versions) == 0 {
		return identifier, errors.New("missing versions")
	}

	var previous *proto.Version
	for i := range versions {
		current := versions[i]
		if current.version.GreaterEqual(version.Minecraft_1_19) {
			return identifier, fmt.Errorf("version %s too old for ID index", current)
		}
		if previous == nil || previous.Lower(current.version) {
			return identifier, errors.New("invalid protocol version order")
		}
		for _, v := range version.Versions {
			if v.GreaterEqual(current.version) {
				identifier.versionByID[v.Protocol] = current.id
			}
		}
		previous = current.version
	}

	return identifier, nil
}

func (v versionSet) String() string {
	return v.version.String()
}
