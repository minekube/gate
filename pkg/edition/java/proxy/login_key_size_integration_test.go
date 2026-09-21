package proxy

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TestOnlineModeLoginCompletesWithConfiguredKeyBits is the end-to-end proof that
// an online mode login completes through a real Gate listener with a login key
// size taken from auth.privateKeyBits, for the vanilla 1024 default and for a
// raised 2048. The client half performs the real handshake: it parses the DER
// encoded public key from the EncryptionRequest, encrypts the 16 byte shared
// secret and the verify token with RSA/PKCS#1 v1.5 (the operations a Minecraft
// client performs), and then decrypts the login stream with AES/CFB8 keyed by
// the shared secret.
//
// The stub session server asserts the hasJoined serverId Gate sends equals
// sha1(sharedSecret || serverPublicKeyDER) as computed independently on the
// client side. That is the value Mojang's real session server validates, so the
// assertion fails if the key the client encrypted against and the key Gate
// derives the server id from ever diverge (the failure mode a key size change
// could introduce).
func TestOnlineModeLoginCompletesWithConfiguredKeyBits(t *testing.T) {
	for _, bits := range []int{1024, 2048} {
		t.Run(fmt.Sprintf("%d_bit_key", bits), func(t *testing.T) {
			runConfiguredKeyBitsLogin(t, bits)
		})
	}
}

func runConfiguredKeyBitsLogin(t *testing.T, bits int) {
	t.Helper()

	const username = "KeySizePlayer"
	profileID := uuid.New()
	profileIDHex := strings.ReplaceAll(profileID.String(), "-", "")

	// --- Mock backend server ---
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend listener: %v", err)
	}
	defer backendListener.Close()
	backendAddr := backendListener.Addr().String()

	var backendDone sync.WaitGroup
	backendDone.Add(1)
	go func() {
		defer backendDone.Done()
		conn, err := backendListener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

		if _, _, err := readPacket(conn); err != nil { // handshake
			t.Errorf("backend: failed to read handshake: %v", err)
			return
		}
		if _, _, err := readPacket(conn); err != nil { // server login
			t.Errorf("backend: failed to read ServerLogin: %v", err)
			return
		}
		if err := writeServerLoginSuccess(conn, profileID, username); err != nil {
			t.Errorf("backend: failed to send LoginSuccess: %v", err)
			return
		}
		if err := writeJoinGame(conn); err != nil {
			t.Errorf("backend: failed to send JoinGame: %v", err)
			return
		}
		time.Sleep(time.Second)
	}()

	// --- Stub session server ---
	type sessionServer struct {
		mu       sync.Mutex
		requests int
		serverID string
	}
	session := &sessionServer{}
	sessionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.mu.Lock()
		session.requests++
		session.serverID = r.URL.Query().Get("serverId")
		session.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"name":%q,"properties":[]}`, profileIDHex, username)
	}))
	defer sessionSrv.Close()

	sessionURL, err := url.Parse(sessionSrv.URL)
	if err != nil {
		t.Fatalf("parse session server url: %v", err)
	}
	sessionKeyURL := configutil.URL(*sessionURL)
	sessionKeyURL.Path = "/session/minecraft/hasJoined"

	// --- Gate proxy ---
	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = true
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1 // no SetCompression frame in the wire assertions
	cfg.Auth.SessionServerURL = &sessionKeyURL
	cfg.Auth.PrivateKeyBits = bits
	cfg.Servers = map[string]string{"lobby": backendAddr}
	cfg.Try = []string{"lobby"}

	p, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	// Proxy goroutines can outlive the test body, so stop logging with it.
	var testDone atomic.Bool
	defer testDone.Store(true)
	p.log = funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{Verbosity: 1})
	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()
	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go p.HandleConn(conn)
		}
	}()
	proxyAddr := proxyListener.Addr().String()

	// --- Client half of the login handshake ---
	client, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("client: failed to connect to proxy: %v", err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))

	host, portStr, _ := net.SplitHostPort(proxyAddr)
	port, _ := strconv.Atoi(portStr)
	if err := writePlainHandshake(client, host, port, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client: failed to send handshake: %v", err)
	}
	if err := writeServerLogin(client, username); err != nil {
		t.Fatalf("client: failed to send ServerLogin: %v", err)
	}

	packetID, data, err := readPacket(client)
	if err != nil {
		t.Fatalf("client: failed to read EncryptionRequest: %v", err)
	}
	if packetID != 0x01 {
		t.Fatalf("client: got packet id 0x%02x, want EncryptionRequest (0x01) for an online mode login", packetID)
	}

	reader := bytes.NewReader(data)
	serverID, err := util.ReadString(reader)
	if err != nil {
		t.Fatalf("client: read EncryptionRequest server id: %v", err)
	}
	if serverID != "" {
		t.Errorf("client: EncryptionRequest server id = %q, want empty", serverID)
	}
	pubKeyDER, err := util.ReadBytes(reader)
	if err != nil {
		t.Fatalf("client: read EncryptionRequest public key: %v", err)
	}
	verifyToken, err := util.ReadBytes(reader)
	if err != nil {
		t.Fatalf("client: read EncryptionRequest verify token: %v", err)
	}

	parsed, err := x509.ParsePKIXPublicKey(pubKeyDER)
	if err != nil {
		t.Fatalf("client: parse server public key: %v", err)
	}
	serverPub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("client: server public key is %T, want *rsa.PublicKey", parsed)
	}
	if got := serverPub.N.BitLen(); got != bits {
		t.Fatalf("client: server login key is %d bits, want %d from auth.privateKeyBits", got, bits)
	}

	// The three crypto operations of the client handshake.
	sharedSecret := make([]byte, 16)
	if _, err := rand.Read(sharedSecret); err != nil {
		t.Fatalf("client: generate shared secret: %v", err)
	}
	encryptedSecret, err := rsa.EncryptPKCS1v15(rand.Reader, serverPub, sharedSecret)
	if err != nil {
		t.Fatalf("client: encrypt shared secret with a %d bit server key: %v", bits, err)
	}
	encryptedToken, err := rsa.EncryptPKCS1v15(rand.Reader, serverPub, verifyToken)
	if err != nil {
		t.Fatalf("client: encrypt verify token with a %d bit server key: %v", bits, err)
	}

	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x01) // EncryptionResponse
	if err := util.WriteBytes(&payload, encryptedSecret); err != nil {
		t.Fatalf("client: write encrypted shared secret: %v", err)
	}
	if err := util.WriteBytes(&payload, encryptedToken); err != nil {
		t.Fatalf("client: write encrypted verify token: %v", err)
	}
	if err := writeFrame(client, payload.Bytes()); err != nil {
		t.Fatalf("client: send EncryptionResponse: %v", err)
	}

	// Everything after EncryptionResponse is AES/CFB8 encrypted with the shared secret.
	decrypted, err := codec.NewDecryptReader(client, sharedSecret)
	if err != nil {
		t.Fatalf("client: enable encryption: %v", err)
	}

	gotLoginSuccess := false
	for i := 0; i < 10 && !gotLoginSuccess; i++ {
		packetID, _, err := readPacket(decrypted)
		if err != nil {
			t.Fatalf("client: read encrypted login stream: %v", err)
		}
		switch packetID {
		case 0x02:
			gotLoginSuccess = true
		case 0x00:
			t.Fatalf("client: disconnected during login instead of completing it")
		}
	}
	if !gotLoginSuccess {
		t.Fatal("client: login did not complete with an encrypted LoginSuccess")
	}

	// The server id Gate authenticated with must be the one the client's
	// handshake produced, or Mojang would reject the join.
	session.mu.Lock()
	requests, gotServerID := session.requests, session.serverID
	session.mu.Unlock()
	if requests != 1 {
		t.Fatalf("session server requests = %d, want 1", requests)
	}
	if want := expectedServerID(sharedSecret, pubKeyDER); gotServerID != want {
		t.Fatalf("hasJoined serverId = %q, want sha1(sharedSecret || serverPublicKey) = %q", gotServerID, want)
	}

	_ = client.Close()
	backendDone.Wait()
}

// expectedServerID mirrors the protocol's server id derivation: the SHA-1 of
// the shared secret followed by the server's public key, with the two's
// complement form for hashes whose high bit is set.
func expectedServerID(sharedSecret, pubKeyDER []byte) string {
	h := sha1.New()
	_, _ = h.Write(sharedSecret)
	_, _ = h.Write(pubKeyDER)
	sum := h.Sum(nil)

	var s strings.Builder
	if sum[0]&0x80 == 0x80 {
		carry := true
		for i := len(sum) - 1; i >= 0; i-- {
			sum[i] = ^sum[i]
			if carry {
				carry = sum[i] == 0xff
				sum[i]++
			}
		}
		s.WriteRune('-')
	}
	s.WriteString(strings.TrimLeft(hex.EncodeToString(sum), "0"))
	return s.String()
}

// writePlainHandshake writes a Handshake packet without a Forge marker.
func writePlainHandshake(w io.Writer, host string, port, protocolVersion int) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Handshake
	pw.VarInt(protocolVersion)
	pw.String(host)
	_ = util.WriteUint16(&payload, uint16(port))
	pw.VarInt(2) // login intent
	return writeFrame(w, payload.Bytes())
}
