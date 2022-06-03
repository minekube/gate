package config

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_loadToken(t *testing.T) {
	t.Run("file should be overwritten when empty", func(t *testing.T) {
		f, err := ioutil.TempFile("", tokenFilename)
		require.NoError(t, err)
		_ = f.Close()
		defer os.Remove(f.Name())

		token, err := loadToken(f.Name())
		require.NoError(t, err)
		require.NotEmpty(t, token)

		token2, err := loadToken(f.Name())
		require.NoError(t, err)
		require.Equal(t, token, token2)
	})

}

func Test_generateSecureToken(t *testing.T) {
	for i := 0; i < 5; i++ {
		require.Len(t, generateSecureToken(i), i)
	}
}
