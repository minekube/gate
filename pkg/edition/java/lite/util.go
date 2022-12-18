package lite

import (
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	forgeSeparator  = "\x00"
	realIPSeparator = "///"
)

// ClearVirtualHost cleans the given virtual host.
func ClearVirtualHost(name string) string {
	name = strings.Split(name, forgeSeparator)[0]  // Remove forge separator
	name = strings.Split(name, realIPSeparator)[0] // Remove real ip separator
	name = strings.Trim(name, ".")
	return name
}

// IsRealIP returns true if the given virtual host uses RealIP protocol.
func IsRealIP(addr string) bool {
	return len(strings.Split(addr, realIPSeparator)) > 1
}

// RealIP formats host addr to use the RealIP protocol with the given client ip.
func RealIP(addr string, clientAddr net.Addr) string {
	addrWithForge := strings.SplitN(addr, forgeSeparator, 3)
	b := new(strings.Builder)

	b.WriteString(addrWithForge[0])
	b.WriteString(realIPSeparator)
	b.WriteString(clientAddr.String())
	b.WriteString(realIPSeparator)
	b.WriteString(strconv.Itoa(int(time.Now().Unix())))
	if len(addrWithForge) > 1 {
		b.WriteString(forgeSeparator)
		b.WriteString(addrWithForge[1])
		b.WriteString(forgeSeparator)
	}

	return b.String()
}
