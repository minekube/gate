package chat

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUnsignedPlayerCommand_Signed(t *testing.T) {
	t.Run("Signed should always return false", func(t *testing.T) {
		u := &UnsignedPlayerCommand{}
		assert.False(t, u.Signed())
		// our intention is guarded by this test
		assert.False(t, u.SessionPlayerCommand.Signed())
	})
}
