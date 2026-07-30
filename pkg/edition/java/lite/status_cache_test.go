package lite

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"golang.org/x/sync/singleflight"
)

func TestResolveStatusResponseRetainsPublicSignature(t *testing.T) {
	var resolve func(
		time.Duration,
		[]liteconfig.Route,
		logr.Logger,
		netmc.MinecraftConn,
		*packet.Handshake,
		*proto.PacketContext,
		*proto.PacketContext,
		*StrategyManager,
	) (logr.Logger, *packet.StatusResponse, error) = ResolveStatusResponse
	_ = resolve
}

func TestPingStatusCacheSeparatesRouteGenerationsAfterReset(t *testing.T) {
	cache := newPingStatusCache(time.Now, new(singleflight.Group))
	oldKey := pingKey{
		backendAddr:     "backend.example.test:25565",
		protocol:        proto.Protocol(765),
		routeGeneration: 1,
	}
	cache.reset()

	old := cache.load(oldKey, time.Minute, func() *pingResult {
		return &pingResult{res: &packet.StatusResponse{Status: "old"}}
	})
	if old.res.Status != "old" {
		t.Fatalf("old route result = %q, want old", old.res.Status)
	}

	newKey := oldKey
	newKey.routeGeneration = 2
	fresh := cache.load(newKey, time.Minute, func() *pingResult {
		return &pingResult{res: &packet.StatusResponse{Status: "new"}}
	})
	if fresh.res.Status != "new" {
		t.Fatalf("new route result = %q, want new", fresh.res.Status)
	}
}
