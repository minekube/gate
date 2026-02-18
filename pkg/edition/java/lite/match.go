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

// FindRouteWithGroups returns the first route that matches the given wildcard supporting pattern,
// along with the captured groups from wildcards.
func FindRouteWithGroups(pattern string, routes ...config.Route) (host string, route *config.Route, groups []string) {
	for i := range routes {
		route = &routes[i]
		for _, host = range route.Host {
			matched, capturedGroups := matchWithGroups(pattern, host)
			if matched {
				return host, route, capturedGroups
			}
		}
	}
	return "", nil, nil
}

// match takes in two strings, s and pattern, and returns a boolean indicating whether s matches pattern.
//
// The following special characters are used in pattern:
//
//	'*': matches any sequence of characters (including an empty sequence)
//	'?': matches any single character
func match(s, pattern string) bool {
	s = strings.ToLower(s)
	pattern = strings.ToLower(pattern)
	reg := compiledRegexCache.Get(pattern)
	return reg != nil && reg.Value() != nil && reg.Value().MatchString(s)
}

var compiledRegexCache = ttlcache.New[string, *regexp.Regexp](
	ttlcache.WithLoader[string, *regexp.Regexp](ttlcache.LoaderFunc[string, *regexp.Regexp](
		func(c *ttlcache.Cache[string, *regexp.Regexp], pattern string) *ttlcache.Item[string, *regexp.Regexp] {
			// pattern is the cache key, we must not modify it for the Set call

			// Escape meta characters to treat them as literals, then restore wildcards
			regexStr := regexp.QuoteMeta(pattern)
			regexStr = "^" + strings.ReplaceAll(regexStr, "\\?", ".") + "$"
			regexStr = strings.ReplaceAll(regexStr, "\\*", ".*")
			reg, _ := regexp.Compile(regexStr)

			return c.Set(pattern, reg, ttlcache.NoTTL)
		}),
	),
)

// matchWithGroups takes in two strings, s and pattern, and returns a boolean indicating whether s matches pattern,
// along with captured groups from wildcards.
//
// The following special characters are used in pattern:
//
//	'*': matches any sequence of characters (including an empty sequence) and captures it as a group
//	'?': matches any single character and captures it as a group
//
// Returns (matched bool, groups []string) where groups contains the captured values in order.
func matchWithGroups(s, pattern string) (bool, []string) {
	s = strings.ToLower(s)
	pattern = strings.ToLower(pattern)

	// Build regex pattern with capture groups
	regexPattern := "^"

	// Track if we're inside a character class (not used in current patterns, but for safety)
	escapeNext := false
	for _, r := range pattern {
		if escapeNext {
			regexPattern += string(r)
			escapeNext = false
			continue
		}

		switch r {
		case '*':
			// Each * becomes a capture group (.*?)
			regexPattern += "(.*?)"
		case '?':
			// Each ? becomes a capture group (.)
			regexPattern += "(.)"
		case '\\':
			// Escape next character
			escapeNext = true
			regexPattern += "\\"
		default:
			// Escape regex special characters
			if strings.ContainsRune(".^$+()[]{}|", r) {
				regexPattern += "\\"
			}
			regexPattern += string(r)
		}
	}

	regexPattern += "$"

	reg, err := regexp.Compile(regexPattern)
	if err != nil {
		// If regex compilation fails, fall back to simple match
		return match(s, pattern), nil
	}

	matches := reg.FindStringSubmatch(s)
	if matches == nil {
		return false, nil
	}

	// matches[0] is the full match, matches[1:] are the capture groups
	if len(matches) > 1 {
		return true, matches[1:]
	}

	return true, []string{}
}
