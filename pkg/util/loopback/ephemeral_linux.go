//go:build linux

package loopback

import (
	"os"
	"strconv"
	"strings"
)

// EphemeralRange returns the host's ephemeral (anonymous) port range, which is
// where every listen-then-close port reservation is drawn from. A dynamically
// allocated, kept listener must stay outside it.
func EphemeralRange() Range {
	raw, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) == 2 {
			start, startErr := strconv.Atoi(fields[0])
			end, endErr := strconv.Atoi(fields[1])
			if startErr == nil && endErr == nil && start > 0 && start <= end {
				return Range{Start: start, End: end}
			}
		}
	}
	return Range{Start: defaultEphemeralStart, End: defaultEphemeralEnd}
}
