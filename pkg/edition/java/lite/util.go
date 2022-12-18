package lite

import "strings"

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
