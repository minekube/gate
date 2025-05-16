package version

import (
	"net/http"
	"strings"
)

// Version information set by build flags
// Version is the current version of Gate.
// Set using -ldflags "-X go.minekube.com/gate/pkg/version.Version=v1.2.3"
var version string = "unknown"

func String() string {
	return version
}

func UserAgent() string {
	s := strings.Builder{}
	s.WriteString("Minekube-Gate/")
	if v := String(); v != "" {
		s.WriteString(v)
	} else {
		s.WriteString("Dirty")
	}
	return s.String()
}

func UserAgentHeader() http.Header {
	h := make(http.Header)
	h.Set("User-Agent", UserAgent())
	return h
}
