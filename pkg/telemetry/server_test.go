package telemetry

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.minekube.com/gate/pkg/gate/config"
)

func TestTracedConnection(t *testing.T) {
	// Create a real TCP connection for testing
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	// Accept connections in a goroutine
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Echo server
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			conn.Write(buf[:n])
		}
	}()

	// Connect to the server
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	// Initialize telemetry using default configuration and enable tracing
	cfg := WithDefaults(&config.Config{})
	cfg.Telemetry.Tracing.Enabled = true
	cfg.Telemetry.Tracing.Exporter = "stdout"
	cleanup, err := initTelemetry(context.Background(), cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Wrap with tracing
	tracedConn := WithConnectionTracing(conn, "test-connection")

	t.Run("read operation", func(t *testing.T) {
		// Write some data
		testData := []byte("hello")
		_, err := tracedConn.Write(testData)
		assert.NoError(t, err)

		// Read it back
		buf := make([]byte, 1024)
		n, err := tracedConn.Read(buf)
		assert.NoError(t, err)
		assert.Equal(t, testData, buf[:n])
	})

	t.Run("write operation", func(t *testing.T) {
		testData := []byte("world")
		n, err := tracedConn.Write(testData)
		assert.NoError(t, err)
		assert.Equal(t, len(testData), n)
	})

	t.Run("close operation", func(t *testing.T) {
		err := tracedConn.Close()
		assert.NoError(t, err)
	})
}

func TestTracedConnectionErrors(t *testing.T) {
	// Test with a closed connection
	_, err := net.Dial("tcp", "localhost:0") // This should fail
	assert.Error(t, err)

	// Still create a traced connection with nil
	tracedConn := WithConnectionTracing(nil, "test-connection")
	assert.Nil(t, tracedConn)
}

func TestConnectionTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	// Create a connection that will timeout
	_, err := net.DialTimeout("tcp", "192.0.2.1:12345", 1*time.Second) // Use an unroutable IP
	assert.Error(t, err)

	// Create traced connection with nil
	tracedConn := WithConnectionTracing(nil, "test-connection")
	assert.Nil(t, tracedConn)
}