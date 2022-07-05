package crypto

import (
	"crypto"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_verifySignature(t *testing.T) {
	const sig = `b0a40bba8d210cd65baf6231c262e030a91ee6e3422fb3101b31554cc686a697218176c948fb0441f1330bc932df2c6baca732ff7c5ec66119b1b3ee7563b09064e84875f5ca1fa1d29d1475793d71a258a3e15ae27538c0f3dc845c9c660b589dea710238f88eb725f0e2465307164d8db78e4f6965859d3bf3db187017766effe180a211e946a5239c01231618eef2aebf492e019d499a807f3b4da5d6b4dd8b473c5668f7a6c1659047c3f97a99b5703f202b11c30282cff172a537f38133c30b48730f72b7ba149f07a512fd73a43c8fd55ee96e29e8c2fd4d31ec423d196c2f7d1cc5db5865259921bc821fec9529a0b2d448765c1e838a97154769e2ef7e4fde656da1fd37bd79cfe63917074edfb5fb7c7c654a63712e614ab2888c2ad974a5586e512ab099c2c1146f11fb5905d37f4e0c373f95062d572977c060c3591c4046bafe6666fdc78526e2380d73c7524314cf1f1a68de0165d455a37b2747c21ae42d279e6a00a4649fafe1f09b981d7ed554f6fb55d25a9d81d06fe44ccf7d0b8b916ffbe50b0051c463ec2c93cfa92a87e157cd66881aeea431408f1558a2419bc5f874ed2054fa366654672fcb56bd1fe4e971686bab61955d0ae884638da91d2a00548ca08526b0958e069a5c88c5e5e6abd7a7edf4ba6f48ded0db61b3e4ec7dc98091b013140dd00c191f661cfe590824818fc1fd71c078cb656b`

	signature, err := hex.DecodeString(sig)
	require.NoError(t, err)

	for i, x := range []struct {
		verified bool
		toVerify string
	}{
		{
			verified: true,
			toVerify: `1656753467181-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxUXtDearoxW4mxLy3FEcNg/7ghAIvCqv
9YyzcWncg/hqdh/YW6f8XpmtwVLs7yvupUTwjXAQzjSPIyViLlvu1klFNbdslVbpsw+64YSm9wA6
v12XF6cZhHmJzOjr67u1wx+nZiQxYxbpBdoH9NZpkbVq+lsNa6D6P/4owZ5s4bQM1MMhk0GNlCM+
CGIUxNbBTei1Sev8fCMrOIWqyaOsWam+2+TOc53NeTkJOjiOsJ2y0mjIxDTpffOvkITjA4g5P3qZ
82DYtcm33pi/P4qcr3tWbHeXFXq8wwTG1IeD0waNHdNtqo7IMC5tjhFpaQa1jgH/bnStqUjZanQ0
b0fKcwIDAQAB
-----END RSA PUBLIC KEY-----
`,
		},
		{
			verified: true,
			toVerify: `1656753467181-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxUXtDearoxW4mxLy3FEcNg/7ghAIvCqv
9YyzcWncg/hqdh/YW6f8XpmtwVLs7yvupUTwjXAQzjSPIyViLlvu1klFNbdslVbpsw+64YSm9wA6
v12XF6cZhHmJzOjr67u1wx+nZiQxYxbpBdoH9NZpkbVq+lsNa6D6P/4owZ5s4bQM1MMhk0GNlCM+
CGIUxNbBTei1Sev8fCMrOIWqyaOsWam+2+TOc53NeTkJOjiOsJ2y0mjIxDTpffOvkITjA4g5P3qZ
82DYtcm33pi/P4qcr3tWbHeXFXq8wwTG1IeD0waNHdNtqo7IMC5tjhFpaQa1jgH/bnStqUjZanQ0
b0fKcwIDAQAB
-----END RSA PUBLIC KEY-----
`,
		},
		{
			verified: false,
			toVerify: `-----BEGIN RSA PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxUXtDearoxW4mxLy3FEcNg/7ghAIvCqv
9YyzcWncg/hqdh/YW6f8XpmtwVLs7yvupUTwjXAQzjSPIyViLlvu1klFNbdslVbpsw+64YSm9wA6
v12XF6cZhHmJzOjr67u1wx+nZiQxYxbpBdoH9NZpkbVq+lsNa6D6P/4owZ5s4bQM1MMhk0GNlCM+
CGIUxNbBTei1Sev8fCMrOIWqyaOsWam+2+TOc53NeTkJOjiOsJ2y0mjIxDTpffOvkITjA4g5P3qZ
82DYtcm33pi/P4qcr3tWbHeXFXq8wwTG1IeD0waNHdNtqo7IMC5tjhFpaQa1jgH/bnStqUjZanQ0
b0fKcwIDAQAB
-----END RSA PUBLIC KEY-----
`,
		},
	} {
		ok := verifySignature(crypto.SHA1, yggdrasilSessionPubKey, signature, []byte(x.toVerify))
		assert.Equal(t, x.verified, ok, "%d", i)
	}

}
