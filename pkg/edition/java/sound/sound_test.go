package sound

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// TestProxyPlayerImplementsSoundPlayer validates that proxy.Player can be used
// as sound.Player, which is required for the sound package examples in the docs.
//
// This test ensures that the usage pattern shown in the documentation examples
// (e.g., .web/docs/developers/examples/sound-example-play.go) will compile.
//
// The sound.Player interface only requires proto.PacketWriter and Protocol(),
// which proxy.Player provides through its embedded interfaces. The additional
// methods (CurrentServerEntityID, CheckServerMatch) are accessed via type
// assertion to internalPlayer in the implementation.
func TestProxyPlayerImplementsSoundPlayer(t *testing.T) {
	// This is a compile-time check: if proxy.Player doesn't implement sound.Player,
	// this will fail to compile.
	var _ Player = (proxy.Player)(nil)
}

// TestSoundPlayWithProxyPlayer validates that sound.Play can accept proxy.Player.
// This mirrors the usage in the documentation examples where:
//   player := e.Player()  // returns proxy.Player
//   sound.Play(player, sound, player)  // should work
//
// The function should compile even though proxy.Player doesn't explicitly
// implement CurrentServerEntityID and CheckServerMatch in its interface.
// The implementation uses type assertion to internalPlayer to access these methods.
func TestSoundPlayWithProxyPlayer(t *testing.T) {
	// This function signature matches what's used in the docs examples.
	// If this compiles, then the docs examples should also compile.
	_ = func(player proxy.Player, snd Sound, emitter Player) error {
		return Play(player, snd, emitter)
	}
}
