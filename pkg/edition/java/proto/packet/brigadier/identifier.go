package brigadier

import (
	"errors"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type (
	ArgumentIdentifier struct {
		id           string
		idByProtocol map[proto.Protocol]int
	}
	versionSet struct {
		version proto.Protocol
		id      int
	}
)

func newArgIdentifier(id string, versions ...versionSet) (*ArgumentIdentifier, error) {
	identifier := &ArgumentIdentifier{
		id:           id,
		idByProtocol: map[proto.Protocol]int{},
	}

	var previous *proto.Protocol
	for i := range versions {
		current := versions[i]
		if !current.version.GreaterEqual(version.Minecraft_1_19) {
			return nil, fmt.Errorf("version %s too old for ID index", current)
		}
		if !(previous == nil || *previous > current.version) {
			return nil, errors.New("invalid protocol version order")
		}
		for _, v := range version.Versions {
			if v.Protocol >= current.version {
				if _, ok := identifier.idByProtocol[v.Protocol]; !ok {
					identifier.idByProtocol[v.Protocol] = current.id
				}
			}
		}
		previous = &current.version
	}

	return identifier, nil
}

func (a ArgumentIdentifier) String() string {
	return a.id
}

func (v versionSet) String() string {
	return v.version.String()
}
