package keyrevision

import "go.minekube.com/gate/pkg/gate/proto"

type Revision interface {
}

var (
	GenericV1 Revision = nil
	LinkedV2  Revision = nil
)

type aKeyRevision struct {
	backwardsCompatibleTo map[Revision]struct{}
	applicableTo          map[*proto.Version]struct{}
}

// GreaterEqual is true when this Revision is
// greater or equal then another Revision.
func GreaterEqual(rev, then Revision) bool {
	if (rev == GenericV1 && then == LinkedV2) || (rev == then) {
		return true
	}

	return false
}
