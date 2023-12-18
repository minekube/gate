package proxy

import (
	"bytes"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/internal/oncetrue"
)

type clientConfigSessionHandler struct {
	player       *connectedPlayer
	brandChannel string

	configSwitchDone oncetrue.OnceWhenTrue
}

// handleBackendFinishUpdate handles the backend finishing the config stage.
func (h *clientConfigSessionHandler) handleBackendFinishUpdate(
	serverConn *serverConnection,
	p *config.FinishedUpdate,
	onConfigSwitch func(),
) {
	smc, ok := serverConn.ensureConnected()
	if ok {
		brand := serverConn.player.ClientBrand()
		if brand == "" && h.brandChannel != "" {
			buf := new(bytes.Buffer)
			_ = util.WriteString(buf, brand)

			brandPacket := &plugin.Message{
				Channel: h.brandChannel,
				Data:    buf.Bytes(),
			}
			_ = smc.WritePacket(brandPacket)
		}
		err := smc.WritePacket(p)
		if err != nil {
			return
		}
	}
	if err := h.player.WritePacket(p); err != nil {
		return
	}

	h.configSwitchDone.DoWhenTrue(onConfigSwitch)
}
