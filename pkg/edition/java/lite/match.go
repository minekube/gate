package lite

import (
	"regexp"
	"strings"

	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
)

// FindRoute returns the first route that matches the given wildcard supporting pattern.
func FindRoute(pattern string, routes ...config.Route) (host string, route *config.Route) {
	for i := range routes {
		route = &routes[i]
		for _, host = range route.Host {
			if match(pattern, host) {
				return host, route
			}
		}
	}
	return "", nil
}

// match takes in two strings, s and pattern, and returns a boolean indicating whether s matches pattern.
//
// The following special characters are used in pattern:
//
//	'*': matches any sequence of characters (including an empty sequence)
//	'?': matches any single character
func match(s, pattern string) bool {
	s = strings.ToLower(s)
	reg := compiledRegexCache.Get(pattern)
	return reg != nil && reg.Value() != nil && reg.Value().MatchString(s)
}

var compiledRegexCache = ttlcache.New[string, *regexp.Regexp](
	ttlcache.WithLoader[string, *regexp.Regexp](ttlcache.LoaderFunc[string, *regexp.Regexp](
		func(c *ttlcache.Cache[string, *regexp.Regexp], pattern string) *ttlcache.Item[string, *regexp.Regexp] {

			pattern = strings.ToLower(pattern)
			pattern = "^" + strings.ReplaceAll(pattern, "?", ".") + "$"
			pattern = strings.ReplaceAll(pattern, "*", ".*")
			reg, _ := regexp.Compile(pattern)

			return c.Set(pattern, reg, ttlcache.NoTTL)
		}),
	),
)
