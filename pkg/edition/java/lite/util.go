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
)

// ClearVirtualHost cleans the given virtual host.
func ClearVirtualHost(name string) string {
	name = strings.Split(name, forgeSeparator)[0]           // Remove forge separator
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
