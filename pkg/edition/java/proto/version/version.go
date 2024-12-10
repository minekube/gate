// Package version contains helpers for working with the Minecraft Java edition versions Gate supports.
package version

import (
	"fmt"
	"strconv"

	"go.minekube.com/gate/pkg/gate/proto"
)

var (
	Unknown          = v(-1, "Unknown")
	Legacy           = v(-2, "Legacy")
	Minecraft_1_7_2  = v(4, "1.7.2", "1.7.3", "1.7.4", "1.7.5")
	Minecraft_1_7_6  = v(5, "1.7.6", "1.7.7", "1.7.8", "1.7.9", "1.7.10")
	Minecraft_1_8    = v(47, "1.8", "1.8.1", "1.8.2", "1.8.3", "1.8.4", "1.8.5", "1.8.6", "1.8.7", "1.8.8", "1.8.9")
	Minecraft_1_9    = v(107, "1.9")
	Minecraft_1_9_1  = v(108, "1.9.1")
	Minecraft_1_9_4  = v(110, "1.9.3", "1.9.4")
	Minecraft_1_10   = v(210, "1.10", "1.10.1", "1.10.2")
	Minecraft_1_11   = v(315, "1.11")
	Minecraft_1_11_1 = v(316, "1.11.1", "1.11.2")
	Minecraft_1_12   = v(335, "1.12")
	Minecraft_1_12_1 = v(338, "1.12.1")
	Minecraft_1_12_2 = v(340, "1.12.2")
	Minecraft_1_13   = v(393, "1.13")
	Minecraft_1_13_2 = v(404, "1.13.2")
	Minecraft_1_14   = v(477, "1.14")
	Minecraft_1_15   = v(573, "1.15")
	Minecraft_1_16   = v(735, "1.16")
	Minecraft_1_16_1 = v(736, "1.16.1")
	Minecraft_1_16_2 = v(751, "1.16.2")
	Minecraft_1_16_3 = v(753, "1.16.3")
	Minecraft_1_16_4 = v(754, "1.16.4", "1.16.5")
	Minecraft_1_17   = v(755, "1.17")
	Minecraft_1_17_1 = v(756, "1.17.1")
	Minecraft_1_18   = v(757, "1.18", "1.18.1")
	Minecraft_1_18_2 = v(758, "1.18.2")
	Minecraft_1_19   = v(759, "1.19")
	Minecraft_1_19_1 = v(760, "1.19.1", "1.19.2")
	Minecraft_1_19_3 = v(761, "1.19.3")
	Minecraft_1_19_4 = v(762, "1.19.4")
	Minecraft_1_20   = v(763, "1.20", "1.20.1")
	Minecraft_1_20_2 = v(764, "1.20.2")
	Minecraft_1_20_3 = v(765, "1.20.3", "1.20.4")
	Minecraft_1_20_5 = v(766, "1.20.5", "1.20.6")
	Minecraft_1_21   = v(767, "1.21", "1.21.1")
	Minecraft_1_21_2 = v(768, "1.21.2", "1.21.3")
	Minecraft_1_21_4 = v(769, "1.21.4")

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
		Minecraft_1_19, Minecraft_1_19_1, Minecraft_1_19_3, Minecraft_1_19_4,
		Minecraft_1_20, Minecraft_1_20_2, Minecraft_1_20_3, Minecraft_1_20_5,
		Minecraft_1_21, Minecraft_1_21_2, Minecraft_1_21_4,
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

func v(protocol proto.Protocol, names ...string) *proto.Version {
	return &proto.Version{Protocol: protocol, Names: names}
}
