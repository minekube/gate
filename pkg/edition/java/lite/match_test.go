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
		{"a", "A*", true},
		{"A", "a*", true},
		{"abc.example.COm", "*.Example.Com", true},
	}

	for _, test := range tests {
		if got := match(test.s, test.pattern); got != test.want {
			t.Errorf("match(%q, %q) = %v, want %v", test.s, test.pattern, got, test.want)
		}
	}
}

func BenchmarkMatch(b *testing.B) {
	s := "Some very long string to match against"
	pattern := "*str?ng*"
	for b.Loop() {
		match(s, pattern)
	}
}
