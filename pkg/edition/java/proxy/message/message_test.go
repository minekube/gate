package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelIdentifierFrom(t *testing.T) {
	for _, x := range []struct {
		id       string
		err      error
		expectID string
		desc     string
	}{
		{"ns:name", nil, "ns:name", "should be valid"},
		{"ns:", ErrNameEmpty, "", "should be empty name"},
		{":name", nil, "minecraft:name", "should be converted to minecraft namespace"},
		{"ns_name", nil, "minecraft:ns_name", "should be valid"},
		{"ns:name:extra", ErrNameInvalid, "", "should be invalid"},
	} {
		i, err := ChannelIdentifierFrom(x.id)
		if assert.ErrorIsf(t, err, x.err, "%s: expected error %v, got (%s, %v)", x.desc, x.err, i, err) && err == nil {
			assert.Equalf(t, x.expectID, i.ID(), "%s: expected id %s, got %s", x.desc, x.expectID, i.ID())
		}
	}
}
