package version

import (
	"net/http"
	"strings"
)

// TODO injected at build time
var version string

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
