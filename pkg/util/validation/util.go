package validation

import (
	"net"
	"regexp"
)

func ValidHostPort(hostAndPort string) error {
	_, _, err := net.SplitHostPort(hostAndPort)
	return err
}

// Constants obtained from https://github.com/kubernetes/apimachinery/blob/master/pkg/util/validation/validation.go
const (
	qnameCharFmt           = "[A-Za-z0-9]"
	qnameExtCharFmt        = "[-A-Za-z0-9_.]"
	qualifiedNameFmt       = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt
	QualifiedNameMaxLength = 63
	QualifiedNameErrMsg    = "must consist of alphanumeric characters, " +
		"'-', '_' or '.', and must start and end with an alphanumeric character"
)

var qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")

func ValidServerName(str string) bool {
	return str != "" && len(str) <= QualifiedNameMaxLength && qualifiedNameRegexp.MatchString(str)
}
