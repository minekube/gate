package lite

import "go.minekube.com/gate/pkg/edition/java/lite/config"

// FindRoute returns the first route that matches the given wildcard supporting pattern.
func FindRoute(pattern string, routes ...config.Route) (host string, ep *config.Route) {
	for i := range routes {
		ep = &routes[i]
		for _, host = range ep.Host {
			if match(pattern, host) {
				return host, ep
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
	// source: https://golangbyexample.com/wildcard-matching-golang/

	runeInput := []rune(s)
	runePattern := []rune(pattern)

	lenInput := len(runeInput)
	lenPattern := len(runePattern)

	isMatchingMatrix := make([][]bool, lenInput+1)

	for i := range isMatchingMatrix {
		isMatchingMatrix[i] = make([]bool, lenPattern+1)
	}

	isMatchingMatrix[0][0] = true
	for i := 1; i < lenInput; i++ {
		isMatchingMatrix[i][0] = false
	}

	if lenPattern > 0 {
		if runePattern[0] == '*' {
			isMatchingMatrix[0][1] = true
		}
	}

	for j := 2; j <= lenPattern; j++ {
		if runePattern[j-1] == '*' {
			isMatchingMatrix[0][j] = isMatchingMatrix[0][j-1]
		}

	}

	for i := 1; i <= lenInput; i++ {
		for j := 1; j <= lenPattern; j++ {

			if runePattern[j-1] == '*' {
				isMatchingMatrix[i][j] = isMatchingMatrix[i-1][j] || isMatchingMatrix[i][j-1]
			}

			if runePattern[j-1] == '?' || runeInput[i-1] == runePattern[j-1] {
				isMatchingMatrix[i][j] = isMatchingMatrix[i-1][j-1]
			}
		}
	}

	return isMatchingMatrix[lenInput][lenPattern]
}
