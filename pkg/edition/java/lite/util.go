package lite

import (
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	forgeSeparator           = "\x00"
	tcpShieldRealIPSeparator = "///"
	// floodgateSeparator is used to separate the original hostname from Bedrock player data
	// This is the same as forgeSeparator but kept explicit for Bedrock handling
	floodgateSeparator = "\x00"
)

// ClearVirtualHost cleans the given virtual host.
// This handles Forge, TCPShield, and Floodgate (Bedrock) separators.
func ClearVirtualHost(name string) string {
	// Remove forge separator (also handles Floodgate since they use the same separator)
	// For Bedrock players via Floodgate, format is: original_hostname\0encrypted_data[:port]
	name = strings.Split(name, forgeSeparator)[0]
	name = strings.Split(name, tcpShieldRealIPSeparator)[0] // Remove real ip separator
	name = strings.Trim(name, ".")
	return name
}

// IsTCPShieldRealIP returns true if the given virtual host uses TCPShieldRealIP protocol.
func IsTCPShieldRealIP(addr string) bool {
	return len(strings.Split(addr, tcpShieldRealIPSeparator)) > 1
}

// TCPShieldRealIP formats host addr to use the TCPShieldRealIP protocol with the given client ip.
func TCPShieldRealIP(addr string, clientAddr net.Addr) string {
	addrWithForge := strings.SplitN(addr, forgeSeparator, 3)
	b := new(strings.Builder)

	b.WriteString(addrWithForge[0])
	b.WriteString(tcpShieldRealIPSeparator)
	b.WriteString(clientAddr.String())
	b.WriteString(tcpShieldRealIPSeparator)
	b.WriteString(strconv.Itoa(int(time.Now().Unix())))
	if len(addrWithForge) > 1 {
		b.WriteString(forgeSeparator)
		b.WriteString(addrWithForge[1])
		b.WriteString(forgeSeparator)
	}

	return b.String()
}
