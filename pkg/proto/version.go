package proto

import (
	"fmt"
	"strconv"
)

var (
	Unknown          = &Version{-1, "Unknown"}
	Legacy           = &Version{-2, "Legacy"}
	Minecraft_1_7_2  = &Version{4, "1.7.2"}
	Minecraft_1_7_6  = &Version{5, "1.7.6"}
	Minecraft_1_8    = &Version{47, "1.8"}
	Minecraft_1_9    = &Version{107, "1.9"}
	Minecraft_1_9_1  = &Version{108, "1.9.1"}
	Minecraft_1_11   = &Version{315, "1.11"}
	Minecraft_1_12   = &Version{335, "1.12"}
	Minecraft_1_12_1 = &Version{338, "1.12.1"}
	Minecraft_1_12_2 = &Version{340, "1.12.2"}
	Minecraft_1_13   = &Version{393, "1.13"}
	Minecraft_1_13_2 = &Version{404, "1.13.2"}
	Minecraft_1_14   = &Version{477, "1.14"}
	Minecraft_1_15   = &Version{573, "1.15"}
	Minecraft_1_16   = &Version{735, "1.16"}
	Minecraft_1_16_1 = &Version{736, "1.16.1"}
	Minecraft_1_16_2 = &Version{751, "1.16.2"}

	// Versions ordered from lowest to highest
	Versions = []*Version{
		Unknown,
		Legacy,
		Minecraft_1_7_2, Minecraft_1_7_6,
		Minecraft_1_8,
		Minecraft_1_9, Minecraft_1_9_1,
		Minecraft_1_11,
		Minecraft_1_12, Minecraft_1_12_1, Minecraft_1_12_2,
		Minecraft_1_13, Minecraft_1_13_2,
		Minecraft_1_14,
		Minecraft_1_15,
		Minecraft_1_16, Minecraft_1_16_1, Minecraft_1_16_2,
	}
)

var (
	IdToVersion = func() map[Protocol]*Version {
		m := make(map[Protocol]*Version, len(Versions))
		for _, v := range Versions {
			m[v.Protocol] = v
		}
		return m
	}()
	SupportedVersions = func() (v []*Version) {
		for _, ver := range Versions {
			if !ver.Unknown() && !ver.Legacy() {
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

// ProtocolVersion gets the Version by the protocol id
// or returns the Unknown version if not found.
func ProtocolVersion(protocol Protocol) *Version {
	v, ok := IdToVersion[protocol]
	if !ok {
		v = Unknown
	}
	return v
}

// Supported returns whether the protocol is supported.
func Supported(protocol Protocol) bool {
	return !protocol.Unknown()
}

// Version is a Minecraft Java version.
type Version struct {
	Protocol
	Name string
}

type Protocol int

func (p Protocol) String() string {
	v := ProtocolVersion(p)
	var s string
	if v == Unknown {
		s = strconv.Itoa(int(p))
	} else {
		s = fmt.Sprintf("%s(%d)", v.Name, p)
	}
	return s
}

func (p Protocol) Supported() bool {
	return !p.Unknown()
}

func (p Protocol) Legacy() bool {
	return p == Legacy.Protocol
}

func (p Protocol) Unknown() bool {
	return p == Unknown.Protocol
}

func (p Protocol) Version() *Version {
	return ProtocolVersion(p)
}

func (v Version) String() string {
	return v.Name
}

func (v *Version) GreaterEqual(then Version) bool {
	return v.Protocol >= then.Protocol
}

func (p Protocol) GreaterEqual(then *Version) bool {
	return p >= then.Protocol
}
func (p Protocol) LowerEqual(then *Version) bool {
	return p <= then.Protocol
}
func (p Protocol) Lower(then *Version) bool {
	return p < then.Protocol
}
func (p Protocol) Greater(then *Version) bool {
	return p > then.Protocol
}
