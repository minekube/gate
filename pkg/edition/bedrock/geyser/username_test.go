package geyser

import (
	"regexp"
	"testing"
)

func TestJavaCompatibleUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "production prefix and Bedrock space", in: ".LLG icedRyan", want: "_LLG_icedRyan"},
		{name: "configured tag", in: "[BE]Player", want: "_BE_Player"},
		{name: "already compatible", in: "Bedrock_Player", want: "Bedrock_Player"},
		{name: "unicode is replaced per rune", in: ".玩家 One", want: "____One"},
		{name: "long name is bounded", in: ".abcdefghijklmnop", want: "_abcdefghijklmno"},
		{name: "empty name stays valid", in: "", want: "_"},
	}

	validJavaUsername := regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := javaCompatibleUsername(tt.in)
			if got != tt.want {
				t.Fatalf("javaCompatibleUsername(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if !validJavaUsername.MatchString(got) {
				t.Fatalf("javaCompatibleUsername(%q) = %q, not a valid Java username", tt.in, got)
			}
		})
	}
}
