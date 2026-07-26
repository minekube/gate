package modernforge

import (
	"strconv"
	"strings"
)

// Token is the token used in the modern forge handshake.
const Token = "FORGE"

// HasToken reports whether the handshake host name carries a null-delimited
// FORGE marker part, as sent by Forge's NetworkContext#enhanceHostName:
// String.join("\0", hostName, "FORGE").
//
// The marker must be its own part: a plain vanilla client connecting to a host
// that merely contains the marker (e.g. "FORGE.example.com") is not a Forge
// client and must not be treated as one.
func HasToken(hostName string) bool {
	parts := strings.Split(hostName, "\000")
	// parts[0] is the host name itself, only the parts after it can be markers.
	for _, pt := range parts[1:] {
		if strings.HasPrefix(pt, Token) {
			return true
		}
	}
	return false
}

// ModernToken aligns the acquisition logic with the internal code of Forge.
// It preserves FML2/FML3 tokens for Forge 1.13-1.20.1, and FORGE tokens for 1.20.2+.
func ModernToken(hostName string) string {
	natVersion := 0
	idx := strings.Index(hostName, "\000")
	if idx != -1 {
		for _, pt := range strings.Split(hostName, "\000") {
			// FML2 (1.13-1.17) and FML3 (1.18-1.20.1) use their own tokens
			// with trailing null bytes as part of the Forge handshake format.
			if strings.HasPrefix(pt, "FML2") || strings.HasPrefix(pt, "FML3") {
				return "\000" + pt + "\000"
			}
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
