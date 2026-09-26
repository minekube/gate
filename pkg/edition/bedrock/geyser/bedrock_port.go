//go:build !musl

package geyser

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// The managed GeyserLite runtime binds the Bedrock UDP listener itself, so a
// second Bedrock listener on the same port on the same host is unrecoverable:
// the runtime starts, fails to bind, and never serves - the RakNet front door
// simply never answers.
//
// Gate mirrors geyserlite's own address derivation below so it can check the
// socket before the runtime is spawned. geyserlite renders its config from the
// Options it is given: Gate leaves Options.Listen unset, so geyserlite falls
// back to a wildcard :19132 baseline, and Options.ConfigOverrides is deep-merged
// over that baseline last (see geyserlite's buildConfigMap). Keep this in step
// with that derivation.
const (
	managedBedrockDefaultAddress = "0.0.0.0"
	managedBedrockDefaultPort    = 19132
)

// BedrockPortUnavailableError reports that the managed Bedrock runtime cannot
// bind the UDP port Bedrock players need, because this host already serves it.
//
// Unlike a runtime failure, this is knowable before the native runtime is
// spawned, so it is reported immediately with an actionable message instead of
// as a startup timeout after liteManagedStartupTimeout - which used to keep the
// Java listener up for ten minutes and then stop the whole proxy, dropping
// every connected Java player over a Bedrock-only misconfiguration.
type BedrockPortUnavailableError struct {
	// Addr is the UDP address the managed runtime needs, e.g. "0.0.0.0:19132".
	Addr string
	// Err is the underlying bind failure, e.g. "address already in use".
	Err error
}

func (e *BedrockPortUnavailableError) Error() string {
	return fmt.Sprintf("managed Bedrock UDP listener %s is already in use by another process on this host "+
		"(Address already in use: %v); stop the other Geyser or Bedrock server (or a previous Gate's "+
		"Geyser that has not exited yet), or set bedrock.managed.configOverrides.bedrock.port to a free port",
		e.Addr, e.Err)
}

func (e *BedrockPortUnavailableError) Unwrap() error { return e.Err }

// managedBedrockListenAddr returns the UDP address the managed runtime will
// bind for Bedrock clients, derived exactly like geyserlite derives its
// bedrock.address/bedrock.port config. It reports false when the overrides
// cannot be modelled, in which case the caller must leave the runtime to
// report its own failure: guessing must never stop a working Gate.
func managedBedrockListenAddr(overrides map[string]any) (string, bool) {
	address := managedBedrockDefaultAddress
	port := managedBedrockDefaultPort

	bedrock, _ := overrides["bedrock"].(map[string]any)
	if raw, ok := bedrock["address"]; ok {
		value, ok := bedrockAddressValue(raw)
		if !ok {
			return "", false
		}
		if value != "" {
			address = value
		}
	}
	if raw, ok := bedrock["port"]; ok {
		value, ok := bedrockPortValue(raw)
		if !ok {
			return "", false
		}
		port = value
	}
	return net.JoinHostPort(address, strconv.Itoa(port)), true
}

// bedrockAddressValue accepts the override spellings a YAML or JSON config can
// produce for an address.
func bedrockAddressValue(raw any) (string, bool) {
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

// bedrockPortValue accepts the override spellings a YAML or JSON config can
// produce for a port (yaml.v3 yields int, encoding/json yields float64).
func bedrockPortValue(raw any) (int, bool) {
	var text string
	switch value := raw.(type) {
	case int:
		text = strconv.Itoa(value)
	case int64:
		text = strconv.FormatInt(value, 10)
	case uint64:
		text = strconv.FormatUint(value, 10)
	case float64:
		if value != math.Trunc(value) {
			return 0, false
		}
		text = strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		text = value.String()
	case string:
		text = strings.TrimSpace(value)
	default:
		return 0, false
	}

	port, err := strconv.Atoi(text)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

// wsaEAddrInUse is WinSock's WSAEADDRINUSE.
const wsaEAddrInUse = syscall.Errno(10048)

// addrInUse reports whether err is the platform's "address already in use"
// bind failure.
//
// errors.Is against syscall.EADDRINUSE covers the Unixes. Windows needs the
// explicit comparison: Go's syscall.EADDRINUSE there is an invented value that
// a real WinSock bind failure does not carry. 10048 is not a valid errno on any
// platform that uses the Unix value, so it cannot misfire there.
func addrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == wsaEAddrInUse
}

// checkBedrockListenPort reports whether the managed runtime can still bind the
// Bedrock UDP listener, so a foreign listener is reported within milliseconds
// instead of as a startup timeout after liteManagedStartupTimeout.
//
// Only EADDRINUSE is treated as a conflict. Any other bind failure (an address
// this host does not own, descriptor limits) is left to the runtime and the
// startup timeout: the probe must never fabricate a port conflict.
//
// The probe binds the socket and closes it again before the runtime starts, so
// it never holds the port the runtime needs. It does not set SO_REUSEADDR, and
// neither does the runtime: measured against the released v0.5.30 native binary
// (2026-09-26), a holder that sets SO_REUSEADDR blocks the runtime exactly like
// one that does not ("Failed to start Geyser on 0.0.0.0:19132", java.net
// BindException: Address in use), while a free port reaches
// "Started Geyser on UDP port 19132". A probe that cannot bind therefore
// cannot disagree with the runtime.
func (r *liteManagedRunner) checkBedrockListenPort() error {
	addr, ok := managedBedrockListenAddr(r.cfg.GetManaged().ConfigOverrides)
	if !ok {
		return nil
	}
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		if addrInUse(err) {
			return &BedrockPortUnavailableError{Addr: addr, Err: err}
		}
		return nil
	}
	// Release immediately: the native runtime, not the probe, owns this port.
	_ = conn.Close()
	return nil
}
