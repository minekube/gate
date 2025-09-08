package lite

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TestHandleFallbackResponse tests the fallback response handling
func TestHandleFallbackResponse(t *testing.T) {
	log := testr.New(t)
	protocol := proto.Protocol(765) // 1.20.4
	backendErr := errors.New("all backends failed")

	t.Run("no fallback configured", func(t *testing.T) {
		route := &config.Route{
			// No fallback
		}

		resp, _ := handleFallbackResponse(log, route, protocol, backendErr)
		assert.Nil(t, resp, "Should return nil when no fallback configured")
	})

	t.Run("nil route", func(t *testing.T) {
		resp, _ := handleFallbackResponse(log, nil, protocol, backendErr)
		assert.Nil(t, resp, "Should return nil for nil route")
	})

	t.Run("fallback with valid MOTD", func(t *testing.T) {
		motd := configutil.TextComponent{
			Content: "Server is down for maintenance",
		}

		route := &config.Route{
			Fallback: &config.Status{
				MOTD: &motd,
				Version: ping.Version{
					Name:     "Maintenance",
					Protocol: 765,
				},
			},
		}

		resp, _ := handleFallbackResponse(log, route, protocol, backendErr)
		require.NotNil(t, resp, "Should return fallback response")

		// Verify the response contains our fallback MOTD
		assert.Contains(t, resp.Status, "Server is down for maintenance", "Response should contain fallback MOTD")
		assert.Contains(t, resp.Status, "Maintenance", "Response should contain version name")
	})

	t.Run("fallback with players info", func(t *testing.T) {
		motd := configutil.TextComponent{
			Content: "All backends offline",
		}

		route := &config.Route{
			Fallback: &config.Status{
				MOTD: &motd,
				Players: &ping.Players{
					Max:    100,
					Online: 0,
					Sample: []ping.SamplePlayer{
						{
							Name: "No servers available",
							ID:   uuid.Nil,
						},
					},
				},
			},
		}

		resp, _ := handleFallbackResponse(log, route, protocol, backendErr)
		require.NotNil(t, resp, "Should return fallback response")

		// Verify players info is in the response
		assert.Contains(t, resp.Status, "\"max\":100", "Response should contain max players")
		assert.Contains(t, resp.Status, "\"online\":0", "Response should contain online players")
		assert.Contains(t, resp.Status, "No servers available", "Response should contain sample player")
	})
}

// TestTryBackendsWithFallback tests the integration of tryBackends with fallback
func TestTryBackendsWithFallback(t *testing.T) {
	log := testr.New(t)

	t.Run("returns errAllBackendsFailed when no backends available", func(t *testing.T) {
		// No backends available
		nextBackend := func() (string, logr.Logger, bool) {
			return "", log, false
		}

		tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
			t.Fatal("Should not be called when no backends")
			return log, nil, nil
		}

		_, _, _, err := tryBackends(nextBackend, tryFunc)
		assert.Equal(t, errAllBackendsFailed, err, "Should return errAllBackendsFailed")
	})

	t.Run("tries all backends before returning error", func(t *testing.T) {
		attempts := 0
		backends := []string{"backend1:25565", "backend2:25565", "backend3:25565"}

		nextBackend := func() (string, logr.Logger, bool) {
			if attempts >= len(backends) {
				return "", log, false
			}
			backend := backends[attempts]
			return backend, log, true
		}

		tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
			attempts++
			// All fail
			return log, nil, errors.New("connection refused")
		}

		_, _, _, err := tryBackends(nextBackend, tryFunc)
		assert.Equal(t, errAllBackendsFailed, err, "Should return errAllBackendsFailed")
		assert.Equal(t, 3, attempts, "Should try all 3 backends")
	})

	t.Run("succeeds on available backend", func(t *testing.T) {
		attempts := 0
		backends := []string{"bad:25565", "good:25565"}

		nextBackend := func() (string, logr.Logger, bool) {
			if attempts >= len(backends) {
				return "", log, false
			}
			backend := backends[attempts]
			return backend, log, true
		}

		successResponse := &packet.StatusResponse{
			Status: `{"description":"Server Online"}`,
		}

		tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
			attempts++
			if backendAddr == "good:25565" {
				return log, successResponse, nil
			}
			return log, nil, errors.New("connection refused")
		}

		backendAddr, _, resp, err := tryBackends(nextBackend, tryFunc)
		assert.NoError(t, err, "Should succeed")
		assert.Equal(t, "good:25565", backendAddr, "Should return successful backend")
		assert.Equal(t, successResponse, resp, "Should return response from good backend")
		assert.Equal(t, 2, attempts, "Should try 2 backends")
	})
}
