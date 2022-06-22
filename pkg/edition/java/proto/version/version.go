// Package version contains helpers for working with the Minecraft Java edition versions Gate supports.
package version

import (
	"fmt"
	"strconv"

	"go.minekube.com/gate/pkg/gate/proto"
)

var (
	Unknown          = &proto.Version{Protocol: -1, Names: s("Unknown")}
	Legacy           = &proto.Version{Protocol: -2, Names: s("Legacy")}
	Minecraft_1_7_2  = &proto.Version{Protocol: 4, Names: s("1.7.2", "1.7.3", "1.7.4", "1.7.5")}
	Minecraft_1_7_6  = &proto.Version{Protocol: 5, Names: s("1.7.6", "1.7.7", "1.7.8", "1.7.9", "1.7.10")}
	Minecraft_1_8    = &proto.Version{Protocol: 47, Names: s("1.8", "1.8.1", "1.8.2", "1.8.3", "1.8.4", "1.8.5", "1.8.6", "1.8.7", "1.8.8", "1.8.9")}
	Minecraft_1_9    = &proto.Version{Protocol: 107, Names: s("1.9")}
	Minecraft_1_9_1  = &proto.Version{Protocol: 108, Names: s("1.9.1")}
	Minecraft_1_9_4  = &proto.Version{Protocol: 110, Names: s("1.9.3", "1.9.4")}
	Minecraft_1_10   = &proto.Version{Protocol: 210, Names: s("1.10", "1.10.1", "1.10.2")}
	Minecraft_1_11   = &proto.Version{Protocol: 315, Names: s("1.11")}
	Minecraft_1_11_1 = &proto.Version{Protocol: 316, Names: s("1.11.1", "1.11.2")}
	Minecraft_1_12   = &proto.Version{Protocol: 335, Names: s("1.12")}
	Minecraft_1_12_1 = &proto.Version{Protocol: 338, Names: s("1.12.1")}
	Minecraft_1_12_2 = &proto.Version{Protocol: 340, Names: s("1.12.2")}
	Minecraft_1_13   = &proto.Version{Protocol: 393, Names: s("1.13")}
	Minecraft_1_13_2 = &proto.Version{Protocol: 404, Names: s("1.13.2")}
	Minecraft_1_14   = &proto.Version{Protocol: 477, Names: s("1.14")}
	Minecraft_1_15   = &proto.Version{Protocol: 573, Names: s("1.15")}
	Minecraft_1_16   = &proto.Version{Protocol: 735, Names: s("1.16")}
	Minecraft_1_16_1 = &proto.Version{Protocol: 736, Names: s("1.16.1")}
	Minecraft_1_16_2 = &proto.Version{Protocol: 751, Names: s("1.16.2")}
	Minecraft_1_16_3 = &proto.Version{Protocol: 753, Names: s("1.16.3")}
	Minecraft_1_16_4 = &proto.Version{Protocol: 754, Names: s("1.16.4", "1.16.5")}
	Minecraft_1_17   = &proto.Version{Protocol: 755, Names: s("1.17")}
	Minecraft_1_17_1 = &proto.Version{Protocol: 756, Names: s("1.17.1")}
	Minecraft_1_18   = &proto.Version{Protocol: 757, Names: s("1.18", "1.18.1")}
	Minecraft_1_18_2 = &proto.Version{Protocol: 758, Names: s("1.18.2")}
	Minecraft_1_19   = &proto.Version{Protocol: 759, Names: s("1.19")}

	// Versions ordered from lowest to highest
	Versions = []*proto.Version{
		Unknown,
		Legacy,
		Minecraft_1_7_2, Minecraft_1_7_6,
		Minecraft_1_8,
		Minecraft_1_9, Minecraft_1_9_1, Minecraft_1_9_4,
		Minecraft_1_10,
		Minecraft_1_11, Minecraft_1_11_1,
		Minecraft_1_12, Minecraft_1_12_1, Minecraft_1_12_2,
		Minecraft_1_13, Minecraft_1_13_2,
		Minecraft_1_14,
		Minecraft_1_15,
		Minecraft_1_16, Minecraft_1_16_1, Minecraft_1_16_2, Minecraft_1_16_3, Minecraft_1_16_4,
		Minecraft_1_17, Minecraft_1_17_1,
		Minecraft_1_18, Minecraft_1_18_2,
		Minecraft_1_19,
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
	// MinimumVersion is the lowest supported version.
	MinimumVersion = SupportedVersions[0]
	// MaximumVersion is the highest supported version.
	MaximumVersion = SupportedVersions[len(SupportedVersions)-1]
	// SupportedVersionsString is the supported versions range as a string.
	SupportedVersionsString = fmt.Sprintf("%s-%s", MinimumVersion, MaximumVersion)
)

// Protocol is proto.Protocol with additional methods for Java edition.
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
		s = fmt.Sprintf("%s(%d)", v.String(), p)
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

// helper func
func s(s ...string) []string { return s }
