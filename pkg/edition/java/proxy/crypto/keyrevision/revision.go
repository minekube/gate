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
