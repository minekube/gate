package proxy

import (
	"regexp"
	"strings"
)

// TODO move to command package
var spaceRegex = regexp.MustCompile(`\s+`)

// trimSpaces removes all spaces that are to much.
func trimSpaces(s string) string {
	s = strings.TrimSpace(s)
	return spaceRegex.ReplaceAllString(s, " ") // remove to much spaces in between
}
