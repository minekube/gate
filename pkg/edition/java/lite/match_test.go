package lite

import "testing"

func Test_match(t *testing.T) {

	tests := []struct {
		s       string
		pattern string
		want    bool
	}{
		{"", "", true},
		{"", "*", true},
		{"", "?", false},
		{"", "a", false},
		{"a", "", false},
		{"a", "*", true},
		{"a", "?", true},
		{"a", "a", true},
		{"a", "b", false},
		{"a", "aa", false},
		{"a", "ab", false},
		{"a", "ba", false},
		{"a", "bb", false},
		{"a", "a?", false},
		{"a", "a*", true},
		{"a", "?a", false},
		{"a", "*a", true},
		{"a", "b?", false},
		{"a", "b*", false},
		{"a", "?b", false},
		{"a", "*b", false},
		{"a", "a*", true},
	}

	for _, test := range tests {
		if got := match(test.s, test.pattern); got != test.want {
			t.Errorf("match(%q, %q) = %v, want %v", test.s, test.pattern, got, test.want)
		}
	}
}
