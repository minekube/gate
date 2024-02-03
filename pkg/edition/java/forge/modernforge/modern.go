package modernforge

import (
	"strconv"
	"strings"
)

// Token is the token used in the modern forge handshake.
const Token = "FORGE"

// ModernToken aligns the acquisition logic with the internal code of Forge.
func ModernToken(hostName string) string {
	natVersion := 0
	idx := strings.Index(hostName, "\000")
	if idx != -1 {
		for _, pt := range strings.Split(hostName, "\000") {
			if strings.HasPrefix(pt, Token) {
				if len(pt) > len(Token) {
					natVersion, _ = strconv.Atoi(pt[len(Token):])
				}
			}
		}
	}
	if natVersion == 0 {
		return "\000" + Token
	}
	return "\000" + Token + strconv.Itoa(natVersion)
}
