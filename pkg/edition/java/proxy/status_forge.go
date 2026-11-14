package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/lite"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

type forgeStatusMetadata struct {
	ModInfo      *modinfo.ModInfo `json:"modinfo"`
	ForgeData    json.RawMessage  `json:"forgeData"`
	NeoForgeData json.RawMessage  `json:"neoforgeData"`
}

// populateForgeStatus enriches Gate's local advertisement before PingEvent.
// Try order determines the advertised mod inventory; metadata from different
// backends must not be combined into a mod list no individual server runs.
func (h *statusSessionHandler) populateForgeStatus(local *ping.ServerPing) {
	cfg := h.proxy.config()
	if !cfg.AnnounceForgeEnabled() {
		return
	}
	var servers []RegisteredServer
	for _, name := range cfg.Try {
		server := h.proxy.Server(name)
		if server == nil {
			continue
		}
		// Join-only dialers need an authenticated Player, which a status
		// connection does not have. Do not bypass their custom transport.
		if _, custom := server.ServerInfo().(ServerDialer); custom {
			continue
		}
		if _, custom := server.(ServerDialer); custom {
			continue
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(h.inbound.Context(), time.Duration(cfg.ConnectionTimeout))
	defer cancel()
	host := ""
	if virtualHost := h.inbound.VirtualHost(); virtualHost != nil {
		host = strings.ToLower(netutil.HostStr(lite.ClearVirtualHost(virtualHost.String())))
	}
	for _, server := range servers {
		if ctx.Err() != nil {
			return
		}
		metadata, err := h.queryForgeStatus(ctx, server.ServerInfo().Addr(), host, local.Version.Protocol, cfg.ProxyProtocolBackend)
		if err != nil {
			h.log.V(1).Info("could not query backend mod list", "server", server.ServerInfo().Name(), "error", err)
			continue
		}
		if metadata.ModInfo == nil && !hasStatusMetadata(metadata.ForgeData) && !hasStatusMetadata(metadata.NeoForgeData) {
			continue
		}
		if metadata.ModInfo != nil {
			if metadata.ModInfo.Mods == nil {
				metadata.ModInfo.Mods = []modinfo.Mod{}
			}
			local.ModInfo = metadata.ModInfo
		}
		for key, data := range map[string]json.RawMessage{"forgeData": metadata.ForgeData, "neoforgeData": metadata.NeoForgeData} {
			if hasStatusMetadata(data) {
				if local.Extra == nil {
					local.Extra = make(map[string]json.RawMessage)
				}
				local.Extra[key] = data
			}
		}
		return
	}
}

func hasStatusMetadata(data json.RawMessage) bool {
	return len(data) != 0 && string(data) != "null"
}

func (h *statusSessionHandler) queryForgeStatus(ctx context.Context, addr net.Addr, host string, protocol proto.Protocol, proxyHeader bool) (*forgeStatusMetadata, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	// DialContext only bounds dialing. Bound writes/reads as well, and wake
	// blocked I/O if the inbound context is canceled before its deadline.
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if proxyHeader {
		if _, err := protoutil.ProxyHeader(h.inbound.RemoteAddr(), conn.RemoteAddr()).WriteTo(conn); err != nil {
			return nil, err
		}
	}
	if host == "" {
		host = netutil.Host(addr)
	}
	enc := codec.NewEncoder(conn, proto.ServerBound, h.log.V(2))
	enc.SetProtocol(protocol)
	if _, err := enc.WritePacket(&packet.Handshake{
		ProtocolVersion: int(protocol), ServerAddress: host,
		Port: int(netutil.Port(addr)), NextStatus: 1,
	}); err != nil {
		return nil, err
	}
	enc.SetState(state.Status)
	if _, err := enc.WritePacket(&packet.StatusRequest{}); err != nil {
		return nil, err
	}
	dec := codec.NewDecoder(conn, proto.ClientBound, h.log.V(2))
	dec.SetProtocol(protocol)
	dec.SetState(state.Status)
	pc, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	response, ok := pc.Packet.(*packet.StatusResponse)
	if !ok {
		return nil, fmt.Errorf("expected status response, got %T", pc.Packet)
	}
	var metadata forgeStatusMetadata
	if err := json.Unmarshal([]byte(response.Status), &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}
