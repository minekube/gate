// Package version contains helpers for working with the Minecraft Java edition versions Gate supports.
package version

import (
	"fmt"
	"go.minekube.com/gate/pkg/gate/proto"
	"strconv"
)

var (
	Unknown          = &proto.Version{Protocol: -1, Name: "Unknown"}
	Legacy           = &proto.Version{Protocol: -2, Name: "Legacy"}
	Minecraft_1_7_2  = &proto.Version{Protocol: 4, Name: "1.7.2"}
	Minecraft_1_7_6  = &proto.Version{Protocol: 5, Name: "1.7.6"}
	Minecraft_1_8    = &proto.Version{Protocol: 47, Name: "1.8"}
	Minecraft_1_9    = &proto.Version{Protocol: 107, Name: "1.9"}
	Minecraft_1_9_1  = &proto.Version{Protocol: 108, Name: "1.9.1"}
	Minecraft_1_9_4  = &proto.Version{Protocol: 110, Name: "1.9.4"}
	Minecraft_1_11   = &proto.Version{Protocol: 315, Name: "1.11"}
	Minecraft_1_12   = &proto.Version{Protocol: 335, Name: "1.12"}
	Minecraft_1_12_1 = &proto.Version{Protocol: 338, Name: "1.12.1"}
	Minecraft_1_12_2 = &proto.Version{Protocol: 340, Name: "1.12.2"}
	Minecraft_1_13   = &proto.Version{Protocol: 393, Name: "1.13"}
	Minecraft_1_13_2 = &proto.Version{Protocol: 404, Name: "1.13.2"}
	Minecraft_1_14   = &proto.Version{Protocol: 477, Name: "1.14"}
	Minecraft_1_15   = &proto.Version{Protocol: 573, Name: "1.15"}
	Minecraft_1_16   = &proto.Version{Protocol: 735, Name: "1.16"}
	Minecraft_1_16_1 = &proto.Version{Protocol: 736, Name: "1.16.1"}
	Minecraft_1_16_2 = &proto.Version{Protocol: 751, Name: "1.16.2"}
	Minecraft_1_16_3 = &proto.Version{Protocol: 753, Name: "1.16.3"}
	Minecraft_1_16_4 = &proto.Version{Protocol: 754, Name: "1.16.4"}

	// Versions ordered from lowest to highest
	Versions = []*proto.Version{
		Unknown,
		Legacy,
		Minecraft_1_7_2, Minecraft_1_7_6,
		Minecraft_1_8,
		Minecraft_1_9, Minecraft_1_9_1, Minecraft_1_9_4,
		Minecraft_1_11,
		Minecraft_1_12, Minecraft_1_12_1, Minecraft_1_12_2,
		Minecraft_1_13, Minecraft_1_13_2,
		Minecraft_1_14,
		Minecraft_1_15,
		Minecraft_1_16, Minecraft_1_16_1, Minecraft_1_16_2, Minecraft_1_16_3, Minecraft_1_16_4,
	}
)

var (
	ProtocolToVersion = func() map[proto.Protocol]*proto.Version {
		m := make(map[proto.Protocol]*proto.Version, len(Versions))
		for _, v := range Versions {
			m[v.Protocol] = v
		}
		return m
	}()
	SupportedVersions = func() (v []*proto.Version) {
		for _, ver := range Versions {
			if !Protocol(ver.Protocol).Unknown() && !Protocol(ver.Protocol).Legacy() {
				v = append(v, ver)
			}
		}
		return
	}()
)

var (
	// The lowest supported version.
	MinimumVersion = SupportedVersions[0]
	// The highest supported version.
	MaximumVersion = SupportedVersions[len(SupportedVersions)-1]
	// The supported versions range as a string.
	SupportedVersionsString = fmt.Sprintf("%s-%s", MinimumVersion, MaximumVersion)
)

// Protocol is a proto.Protocol with additional methods for Java edition.
type Protocol proto.Protocol

// Version gets the Version by the protocol id
// or returns the Unknown version if not found.
func (p Protocol) Version() *proto.Version {
	v, ok := ProtocolToVersion[proto.Protocol(p)]
	if !ok {
		v = Unknown
	}
	return v
}

func (p Protocol) String() string {
	v := p.Version()
	var s string
	if v == Unknown {
		s = strconv.Itoa(int(p))
	} else {
		s = fmt.Sprintf("%s(%d)", v.Name, p)
	}
	return s
}

// Supported returns true if the protocol is a supported Minecraft Java edition version.
func (p Protocol) Supported() bool {
	return !p.Unknown()
}

func (p Protocol) Legacy() bool {
	return proto.Protocol(p) == Legacy.Protocol
}

func (p Protocol) Unknown() bool {
	return proto.Protocol(p) == Unknown.Protocol
}
