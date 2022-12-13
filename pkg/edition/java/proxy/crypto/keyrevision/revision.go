package keyrevision

import (
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type Revision interface {
	ApplicableTo() []proto.Protocol
}

var (
	GenericV1 Revision = &revision{
		name: "GenericV1",
		applicableTo: []proto.Protocol{
			version.Minecraft_1_19.Protocol,
		},
	}
	LinkedV2 Revision = &revision{
		name: "LinkedV2",
		applicableTo: []proto.Protocol{
			version.Minecraft_1_19_1.Protocol,
		},
	}
	Revisions = []Revision{
		GenericV1,
		LinkedV2,
	}
)

// Applicable returns whether the revision is applicable to the protocol version.
func Applicable(rev Revision, protocol proto.Protocol) bool {
	for _, p := range rev.ApplicableTo() {
		if p == protocol {
			return true
		}
	}
	return false
}

// RevisionIndex returns the index of the revision in the Revisions slice.
func RevisionIndex(rev Revision) int {
	for i, r := range Revisions {
		if r == rev {
			return i
		}
	}
	return -1
}

type revision struct {
	applicableTo []proto.Protocol
	name         string
}

func (r *revision) ApplicableTo() []proto.Protocol {
	return r.applicableTo
}

func (r *revision) String() string {
	return r.name
}
