package validation

import (
	"net"
	"regexp"
)

func ValidHostPort(hostAndPort string) error {
	_, _, err := net.SplitHostPort(hostAndPort)
	return err
}

// Constants obtained from https://github.com/kubernetes/apimachinery/blob/master/pkg/util/validation/validation.go
const (
	qnameCharFmt           = "[A-Za-z0-9]"
	qnameExtCharFmt        = "[-A-Za-z0-9_.]"
	qualifiedNameFmt       = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt
	QualifiedNameMaxLength = 63
	QualifiedNameErrMsg    = "must consist of alphanumeric characters, " +
		"'-', '_' or '.', and must start and end with an alphanumeric character"
)

var qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")

func ValidServerName(str string) bool {
	return str != "" && len(str) <= QualifiedNameMaxLength && qualifiedNameRegexp.MatchString(str)
}

// IsAllowedCharacter checks if c is a allowed character in Minecraft chat.
func IsAllowedCharacter(c rune) bool {
	// 167 = §, 127 = DEL
	// https://minecraft.fandom.com/wiki/Multiplayer#Chat
	return c != 167 && c >= ' ' && c != 127
}

// ContainsIllegalCharacter check if a message contains illegal characters in Minecraft chat.
//
// It is not possible to send certain characters in the chat like:
// section symbol, DEL, and all characters below space.
func ContainsIllegalCharacter(s string) bool {
	for _, c := range s {
		if !IsAllowedCharacter(c) {
			return true
		}
	}
	return false
}
