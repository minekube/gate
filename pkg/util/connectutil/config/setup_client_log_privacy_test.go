package config

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/connect"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type fakeProposal struct {
	session *connect.Session

	mu       sync.Mutex
	rejected []*connect.RejectionReason
}

func (f *fakeProposal) Session() *connect.Session { return f.session }

func (f *fakeProposal) Reject(_ context.Context, reason *connect.RejectionReason) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejected = append(f.rejected, reason)
	return nil
}

func (f *fakeProposal) rejections() []*connect.RejectionReason {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*connect.RejectionReason(nil), f.rejected...)
}

// capturingLogger records every log line, including verbose ones, so the
// tests can prove no identity material reaches logs at any verbosity.
func capturingLogger() (logr.Logger, func() string) {
	var mu sync.Mutex
	var lines []string
	log := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, prefix+" "+args)
	}, funcr.Options{Verbosity: 10})
	return log, func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(lines, "\n")
	}
}

// sessionWithEnvelope appends the frozen v2 session fields to the marshaled
// session the way an up-to-date Connect edge sends them.
func sessionWithEnvelope(t *testing.T, s *connect.Session, envelope []byte) *connect.Session {
	t.Helper()
	raw, err := proto.Marshal(s)
	require.NoError(t, err)
	raw = protowire.AppendTag(raw, 6, protowire.VarintType) // protocol
	raw = protowire.AppendVarint(raw, 2)                    // SESSION_PROTOCOL_BEDROCK
	raw = protowire.AppendTag(raw, 7, protowire.BytesType)
	raw = protowire.AppendString(raw, "endpoint-1")
	raw = protowire.AppendTag(raw, 8, protowire.BytesType)
	raw = protowire.AppendString(raw, "org-1")
	raw = protowire.AppendTag(raw, 9, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte("0123456789abcdef"))
	raw = protowire.AppendTag(raw, 10, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 3)
	raw = protowire.AppendTag(raw, 11, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 7)
	raw = protowire.AppendTag(raw, 12, protowire.BytesType)
	raw = protowire.AppendBytes(raw, envelope)
	out := new(connect.Session)
	require.NoError(t, proto.Unmarshal(raw, out))
	return out
}

func TestProposalHandlingLogsNoIdentityMaterial(t *testing.T) {
	const (
		sentinelUsername = "SENTINEL_USERNAME"
		sentinelAddr     = "sentinel.player.addr.example:19132"
		sentinelUUID     = "d5f5a381-70a5-4837-a6ca-adbb4a83a223"
	)
	log, capture := capturingLogger()
	ctx := logr.NewContext(context.Background(), log)

	ph := &proposalHandler{connHandler: func(net.Conn) {}}
	proposal := &fakeProposal{session: &connect.Session{
		Id: "sess-privacy-1",
		// Missing tunnel service address rejects the proposal after the
		// logger already carries its proposal-scoped values.
		Player: &connect.Player{
			Addr: sentinelAddr,
			Profile: &connect.GameProfile{
				Id:   sentinelUUID,
				Name: sentinelUsername,
			},
		},
	}}
	ph.handle(ctx, proposal)
	require.Len(t, proposal.rejections(), 1)

	logs := capture()
	require.Contains(t, logs, "sess-privacy-1")
	for _, forbidden := range []string{sentinelUsername, sentinelAddr, sentinelUUID} {
		require.NotContains(t, logs, forbidden)
	}
}

func TestPrincipalProposalRejectionLogsOnlyBoundedCategory(t *testing.T) {
	const envelopeSentinel = "RAW_ENVELOPE_SENTINEL_MATERIAL"
	log, capture := capturingLogger()
	ctx := logr.NewContext(context.Background(), log)

	pub, _ := testKeyPair(t)
	principal, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	ph := &proposalHandler{connHandler: func(net.Conn) {}, principal: principal}
	proposal := &fakeProposal{session: sessionWithEnvelope(t, &connect.Session{
		Id:                "sess-privacy-2",
		TunnelServiceAddr: "wss://tunnel.invalid",
		Player:            &connect.Player{Addr: "player.addr.example:19132"},
	}, []byte(envelopeSentinel))}
	ph.handle(ctx, proposal)

	rejections := proposal.rejections()
	require.Len(t, rejections, 1)
	require.Contains(t, rejections[0].GetMessage(), "MALFORMED")
	require.NotContains(t, rejections[0].GetMessage(), envelopeSentinel)

	logs := capture()
	require.Contains(t, logs, "MALFORMED")
	require.NotContains(t, logs, envelopeSentinel)
}

func TestBedrockProposalWithoutEnvelopeIsRejectedInRequireMode(t *testing.T) {
	log, capture := capturingLogger()
	ctx := logr.NewContext(context.Background(), log)

	pub, _ := testKeyPair(t)
	principal, err := newPrincipalVerifier(testPrincipalConfig(pub))
	require.NoError(t, err)

	ph := &proposalHandler{connHandler: func(net.Conn) {}, principal: principal}
	session := &connect.Session{
		Id:                "sess-privacy-3",
		TunnelServiceAddr: "wss://tunnel.invalid",
		Player: &connect.Player{
			Addr:    "player.addr.example:19132",
			Profile: &connect.GameProfile{Id: "d5f5a381-70a5-4837-a6ca-adbb4a83a223", Name: "SomeName"},
		},
	}
	// Bedrock protocol declared, but no signed principal envelope.
	raw, err := proto.Marshal(session)
	require.NoError(t, err)
	raw = protowire.AppendTag(raw, 6, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 2)
	bedrockSession := new(connect.Session)
	require.NoError(t, proto.Unmarshal(raw, bedrockSession))

	ph.handle(ctx, &fakeProposal{session: bedrockSession})
	// The handler owns rejection; the proposal must not be admitted on the
	// proposed profile alone.
	require.Contains(t, capture(), "missing a signed principal envelope")
}
