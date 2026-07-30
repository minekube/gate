package addrquota

import "testing"

// TestIPKeyBucketing pins the quota bucket sizes: IPv4 /24 and IPv6 /64.
// The IPv6 cases fail against a narrower mask (e.g. zeroing only the final
// byte, a /120), which would let a single /64 subscriber rotate through 2^56
// addresses and evade the limiter.
func TestIPKeyBucketing(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		sameKey bool
	}{
		{"ipv6 same /64", "2001:db8:1:2::1", "2001:db8:1:2:ffff:ffff:ffff:ffff", true},
		{"ipv6 same /64, differing above the low byte", "2001:db8:1:2::1", "2001:db8:1:2:aaaa:bbbb:cccc:1", true},
		{"ipv6 different /64", "2001:db8:1:2::1", "2001:db8:1:3::1", false},
		{"ipv6 different /64, identical low 64 bits", "2001:db8:1:2:aaaa::1", "2001:db8:9:9:aaaa::1", false},
		{"ipv4 same /24", "203.0.113.7", "203.0.113.200", true},
		{"ipv4 different /24", "203.0.113.7", "203.0.114.7", false},
		{"ipv4-mapped ipv6 matches its ipv4 /24", "::ffff:203.0.113.7", "203.0.113.200", true},
		{"ipv4-mapped ipv6 different /24", "::ffff:203.0.113.7", "::ffff:203.0.114.7", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka, kb := ipKey(tt.a), ipKey(tt.b)
			if ka == "" || kb == "" {
				t.Fatalf("ipKey returned empty key: ipKey(%q)=%q, ipKey(%q)=%q", tt.a, ka, tt.b, kb)
			}
			if got := ka == kb; got != tt.sameKey {
				t.Errorf("ipKey(%q)=%q, ipKey(%q)=%q: same bucket = %v, want %v",
					tt.a, ka, tt.b, kb, got, tt.sameKey)
			}
		})
	}
}

func TestIPKeyPrefixes(t *testing.T) {
	tests := []struct{ ip, want string }{
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2::"},
		{"::1", "::"},
		{"203.0.113.7", "203.0.113.0"},
		{"::ffff:203.0.113.7", "203.0.113.0"},
		{"not an ip", ""},
	}
	for _, tt := range tests {
		if got := ipKey(tt.ip); got != tt.want {
			t.Errorf("ipKey(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}
