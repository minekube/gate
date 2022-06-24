package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelIdentifierFrom(t *testing.T) {
	for _, x := range []struct {
		id  string
		err error
	}{
		{"ns:name", nil},
		{"ns:", errIdentifierEmpty},
		{"ns_name", errIdentifierNoColon},
	} {
		i, err := ChannelIdentifierFrom(x.id)
		assert.ErrorIs(t, err, x.err)
		if err == nil {
			assert.Equal(t, x.id, i.ID())
		}
	}
}
