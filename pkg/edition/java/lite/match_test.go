package lite

import (
	"slices"
	"testing"
)

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

func Test_matchWithGroups(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		pattern    string
		want       bool
		wantGroups []string
	}{
		// Single wildcard tests
		{
			name:       "single star matches subdomain",
			s:          "abc.domain.com",
			pattern:    "*.domain.com",
			want:       true,
			wantGroups: []string{"abc"},
		},
		{
			name:       "single star does not match when dot is required",
			s:          "domain.com",
			pattern:    "*.domain.com",
			want:       false,
			wantGroups: nil,
		},
		{
			name:       "single star at start",
			s:          "abc.example.com",
			pattern:    "*.example.com",
			want:       true,
			wantGroups: []string{"abc"},
		},
		{
			name:       "single star at end",
			s:          "example.com",
			pattern:    "example.*",
			want:       true,
			wantGroups: []string{"com"},
		},
		{
			name:       "single star in middle",
			s:          "abc.example.com",
			pattern:    "abc.*.com",
			want:       true,
			wantGroups: []string{"example"},
		},

		// Multiple wildcard tests
		{
			name:       "two stars capture two groups",
			s:          "abc.def.com",
			pattern:    "*.*.com",
			want:       true,
			wantGroups: []string{"abc", "def"},
		},
		{
			name:       "three stars capture three groups",
			s:          "a.b.c.example.com",
			pattern:    "*.*.*.example.com",
			want:       true,
			wantGroups: []string{"a", "b", "c"},
		},
		{
			name:       "mixed wildcards in pattern",
			s:          "sub.example.com",
			pattern:    "*.example.*",
			want:       true,
			wantGroups: []string{"sub", "com"},
		},

		// Question mark wildcard tests
		{
			name:       "single question mark matches one char",
			s:          "a.example.com",
			pattern:    "?.example.com",
			want:       true,
			wantGroups: []string{"a"},
		},
		{
			name:       "question mark doesn't match empty",
			s:          ".example.com",
			pattern:    "?.example.com",
			want:       false,
			wantGroups: nil,
		},
		{
			name:       "question mark doesn't match multiple chars",
			s:          "ab.example.com",
			pattern:    "?.example.com",
			want:       false,
			wantGroups: nil,
		},
		{
			name:       "mixed star and question mark",
			s:          "a.example.com",
			pattern:    "?.example.*",
			want:       true,
			wantGroups: []string{"a", "com"},
		},

		// Edge cases
		{
			name:       "no wildcards returns empty groups",
			s:          "example.com",
			pattern:    "example.com",
			want:       true,
			wantGroups: []string{},
		},
		{
			name:       "empty string with star",
			s:          "",
			pattern:    "*",
			want:       true,
			wantGroups: []string{""},
		},
		{
			name:       "empty string with question mark",
			s:          "",
			pattern:    "?",
			want:       false,
			wantGroups: nil,
		},
		{
			name:       "no match returns false",
			s:          "abc.example.com",
			pattern:    "xyz.example.com",
			want:       false,
			wantGroups: nil,
		},
		{
			name:       "no match with wildcard",
			s:          "abc.example.com",
			pattern:    "*.other.com",
			want:       false,
			wantGroups: nil,
		},

		// Case insensitivity
		{
			name:       "case insensitive matching",
			s:          "ABC.DOMAIN.COM",
			pattern:    "*.domain.com",
			want:       true,
			wantGroups: []string{"abc"},
		},
		{
			name:       "case insensitive pattern",
			s:          "abc.domain.com",
			pattern:    "*.DOMAIN.COM",
			want:       true,
			wantGroups: []string{"abc"},
		},
		{
			name:       "mixed case",
			s:          "AbC.ExAmPlE.CoM",
			pattern:    "*.example.*",
			want:       true,
			wantGroups: []string{"abc", "com"},
		},

		// Complex patterns
		{
			name:       "multiple stars in sequence",
			s:          "a.b.c.d.example.com",
			pattern:    "*.*.*.*.example.com",
			want:       true,
			wantGroups: []string{"a", "b", "c", "d"},
		},
		{
			name:       "star matches multiple segments",
			s:          "very.long.subdomain.example.com",
			pattern:    "*.example.com",
			want:       true,
			wantGroups: []string{"very.long.subdomain"},
		},
		{
			name:       "real world example",
			s:          "abc.domain.com",
			pattern:    "*.domain.com",
			want:       true,
			wantGroups: []string{"abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotGroups := matchWithGroups(tt.s, tt.pattern)
			if got != tt.want {
				t.Errorf("matchWithGroups(%q, %q) match = %v, want %v", tt.s, tt.pattern, got, tt.want)
			}
			if !slices.Equal(gotGroups, tt.wantGroups) {
				t.Errorf("matchWithGroups(%q, %q) groups = %v, want %v", tt.s, tt.pattern, gotGroups, tt.wantGroups)
			}
		})
	}
}
