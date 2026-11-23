package forge

import (
	"bytes"
	"reflect"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

func TestReadModsPreservesHandshakePayload(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload []byte
		mods    []modinfo.Mod
		wantErr bool
	}{
		{"mod list", []byte{2, 1, 3, 'f', 'm', 'l', 3, '1', '.', '0'}, []modinfo.Mod{{ID: "fml", Version: "1.0"}}, false},
		{"empty mod list", []byte{2, 0}, []modinfo.Mod{}, false},
		{"trailing bytes", []byte{2, 0, 0xab, 0xcd}, []modinfo.Mod{}, false},
		{"truncated mod list", []byte{2, 1, 3, 'f'}, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			msg := &plugin.Message{Channel: LegacyHandshakeChannel, Data: bytes.Clone(tt.payload)}
			mods, err := ReadMods(msg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadMods error = %v, want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(mods, tt.mods) {
				t.Errorf("mods = %#v, want %#v", mods, tt.mods)
			}
			// The caller forwards this same packet to the backend after inspection.
			if msg.Channel != LegacyHandshakeChannel || !bytes.Equal(msg.Data, tt.payload) {
				t.Errorf("handshake packet changed: channel=%q payload=%x, want payload=%x", msg.Channel, msg.Data, tt.payload)
			}
		})
	}
}
