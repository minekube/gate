package examples

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/robinbraemer/event"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

// call this in your plugin init function
func initPlayerInfo(p *proxy.Proxy) {
	subscribeJoinTime(p)
	registerPlayerInfoCommand(p)
}

func registerPlayerInfoCommand(p *proxy.Proxy) {
	p.Command().Register(playerInfoCommand())
}

func playerInfoCommand() brigodier.LiteralNodeBuilder {
	return brigodier.Literal("playerinfo").
		Executes(command.Command(func(c *command.Context) error {
			player, ok := c.Source.(proxy.Player)
			if !ok {
				return c.SendMessage(&component.Text{Content: "Only players can use this command."})
			}
			return c.SendMessage(playerInfo(player))
		}))
}

func playerInfo(p proxy.Player) *component.Text {
	state.RLock()
	joinTime := state.joinTime[p.ID()]
	state.RUnlock()

	msg := fmt.Sprintf("Your name is %s and you joined %s",
		p.Username(), time.Since(joinTime).Round(time.Second))

	return &component.Text{Content: msg}
}

func subscribeJoinTime(p *proxy.Proxy) {
	event.Subscribe(p.Event(), math.MaxInt, saveJoinTime)
	event.Subscribe(p.Event(), math.MinInt, deleteJoinTime)
}

var state = struct {
	joinTime map[uuid.UUID]time.Time
	sync.RWMutex
}{
	joinTime: make(map[uuid.UUID]time.Time),
}

func saveJoinTime(e *proxy.PostLoginEvent) {
	state.Lock()
	defer state.Unlock()
	state.joinTime[e.Player().ID()] = time.Now()
}

func deleteJoinTime(e *proxy.DisconnectEvent) {
	state.Lock()
	defer state.Unlock()
	delete(state.joinTime, e.Player().ID())
}
