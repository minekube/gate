package crypto

import (
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parse_yggdrasilSessionPubKey(t *testing.T) {
	pk, err := x509.ParsePKCS1PublicKey(yggdrasilSessionPubKeyDER)
	require.NoError(t, err)
	require.NotNil(t, pk)
}
