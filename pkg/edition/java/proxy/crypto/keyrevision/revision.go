package keyrevision

import "go.minekube.com/gate/pkg/gate/proto"

type Revision interface {
	ApplicableTo() []proto.Protocol
}

var (
	GenericV1 Revision = nil
	LinkedV2  Revision = nil
)

type aKeyRevision struct {
	backwardsCompatibleTo []Revision
	applicableTo          []proto.Protocol
}

// Applicable returns whether the revision is applicable to the protocol version.
func Applicable(rev Revision, protocol proto.Protocol) bool {
	for _, p := range rev.ApplicableTo() {
		if p == protocol {
			return true
		}
	}
	return false
}
