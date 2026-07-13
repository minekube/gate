package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestLiteRoutesChanged(t *testing.T) {
	base := []liteconfig.Route{{
		Host:         []string{"play.example.com"},
		Backend:      []string{"backend.example:25565"},
		CachePingTTL: configutil.Duration(30 * time.Second),
	}}

	tests := []struct {
		name     string
		current  []liteconfig.Route
		previous []liteconfig.Route
		want     bool
	}{
		{name: "unchanged routes", current: base, previous: base, want: false},
		{
			name:     "backend changed",
			current:  []liteconfig.Route{{Host: []string{"play.example.com"}, Backend: []string{"new-backend.example:25565"}, CachePingTTL: configutil.Duration(30 * time.Second)}},
			previous: base,
			want:     true,
		},
		{
			name:     "cache ttl changed",
			current:  []liteconfig.Route{{Host: []string{"play.example.com"}, Backend: []string{"backend.example:25565"}, CachePingTTL: configutil.Duration(time.Minute)}},
			previous: base,
			want:     true,
		},
		{name: "route added", current: append(base, liteconfig.Route{Host: []string{"other.example.com"}, Backend: []string{"other.example:25565"}}), previous: base, want: true},
		{name: "route removed", current: base, previous: append(base, liteconfig.Route{Host: []string{"other.example.com"}, Backend: []string{"other.example:25565"}}), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, liteRoutesChanged(tt.current, tt.previous))
		})
	}
}
