package crypto

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/ascii85"
	"fmt"
	"hash"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/util/uuid"
)

// IdentifiedKey represents session-server cross-signed dated RSA public key.
type IdentifiedKey interface {
	KeySigned
	// SignedPublicKey returns RSA public key.
	// Note: this key is at least 2048 bits but may be larger.
	SignedPublicKey() *rsa.PublicKey
	// VerifyDataSignature validates a signature against this public key.
	VerifyDataSignature(signature []byte, toVerify ...[]byte) bool
}

// KeyIdentifiable identifies a type with a public RSA signature.
type KeyIdentifiable interface {
	// IdentifiedKey returns the timed identified key of the object context.
	// Only available in 1.19 and newer.
	IdentifiedKey() IdentifiedKey
}

type KeySigned interface {
	Signer() *rsa.PublicKey

	// ExpiryTemporal returns the expiry time point of the key.
	// Note: this limit is arbitrary. RSA keys don't expire,
	// but the signature of this key as provided by the session
	// server will expire.
	ExpiryTemporal() time.Time

	// Expired checks if the signature has expired.
	Expired() bool

	// Signature retrieves the RSA signature of the signed object.
	Signature() []byte

	// SignatureValid validates the signature, expiry temporal and key against the signer public key.
	//
	// Note: This will not check for expiry.
	//
	// DOES NOT WORK YET FOR MESSAGES AND COMMANDS!
	SignatureValid() bool

	// Salt returns the signature salt or empty if not salted.
	Salt() []byte
}

type SignedMessage interface {
	KeySigned
	Message() string       // Returns the signed message.
	SignerUUID() uuid.UUID // Returns the signers UUID.
	PreviewSigned() bool   // If true the signature of this message applies to a stylized component instead.
}

type identifiedKey struct {
	p              *rsa.PublicKey
	signature      []byte
	expiryTemporal time.Time

	once struct {
		sync.Once
		isSignatureValid bool
		err              error
	}
}

var _ IdentifiedKey = (*identifiedKey)(nil)

func NewIdentifiedKey(key []byte, expiry int64, signature []byte) (IdentifiedKey, error) {
	pk, err := x509.ParsePKCS1PublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("error parse public key: %w", err)
	}
	return &identifiedKey{
		p:              pk,
		signature:      signature,
		expiryTemporal: time.UnixMilli(expiry),
	}, nil
}

//go:embed yggdrasil_session_pubkey.der
var yggdrasilSessionPubKeyDER []byte

var yggdrasilSessionPubKey *rsa.PublicKey

func init() {
	yggdrasilSessionPubKey, _ = x509.ParsePKCS1PublicKey(yggdrasilSessionPubKeyDER)
}

func (i *identifiedKey) Signer() *rsa.PublicKey {
	return yggdrasilSessionPubKey
}

func (i *identifiedKey) ExpiryTemporal() time.Time {
	return i.expiryTemporal
}

func (i *identifiedKey) Expired() bool {
	return time.Now().After(i.expiryTemporal)
}

func (i *identifiedKey) Signature() []byte {
	return i.signature
}

func (i *identifiedKey) Salt() []byte {
	return nil
}

func (i *identifiedKey) SignedPublicKey() *rsa.PublicKey {
	return i.p
}

func (i *identifiedKey) SignatureValid() bool {
	i.once.Do(func() {
		pemKey, err := generatePublicPemFromKey(i.p)
		if err != nil {
			i.once.err = err
			return
		}
		expires := i.expiryTemporal.UnixMilli()
		toVerify := new(bytes.Buffer)
		asciiEnc := ascii85.NewEncoder(toVerify)
		_, err = asciiEnc.Write([]byte(fmt.Sprintf("%d%s", expires, pemKey)))
		if err != nil {
			i.once.err = err
			return
		}
		if err = asciiEnc.Close(); err != nil {
			i.once.err = err
			return
		}
		i.once.isSignatureValid = verifySignature(crypto.SHA1, i.p,
			yggdrasilSessionPubKeyDER, sha1.New(), toVerify.Bytes())
	})
	return i.once.isSignatureValid
}

func (i *identifiedKey) VerifyDataSignature(signature []byte, toVerify ...[]byte) bool {
	return verifySignature(crypto.SHA256, i.p, signature, sha256.New(), toVerify...)
}

func verifySignature(algorithm crypto.Hash, key *rsa.PublicKey, signature []byte, hash hash.Hash, toVerify ...[]byte) bool {
	if len(toVerify) == 0 {
		return false
	}
	for _, b := range toVerify {
		_, _ = hash.Write(b)
	}
	return nil == rsa.VerifyPKCS1v15(key, algorithm, hash.Sum(nil), signature)
}

// Equal checks whether a and b are equal.
func Equal(a, b IdentifiedKey) bool {
	if a == b {
		return true
	}
	return a.SignedPublicKey().Equal(b.SignedPublicKey()) &&
		a.ExpiryTemporal().Equal(b.ExpiryTemporal()) &&
		bytes.Equal(a.Signature(), b.Signature()) &&
		a.Signer().Equal(b.Signer())
}

func generatePublicPemFromKey(publicKey *rsa.PublicKey) (string, error) {
	encoded, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	b := new(strings.Builder)
	b.WriteString("-----BEGIN RSA PUBLIC KEY-----\n")
	b.WriteString(string(encoded))
	b.WriteRune('\n')
	b.WriteString("-----END RSA PUBLIC KEY-----\n")
	return b.String(), nil
}

type (
	SignedChatMessage struct {
		Message       string
		Signer        *rsa.PublicKey
		Signature     []byte
		Expiry        time.Time
		Salt          []byte
		Sender        uuid.UUID
		SignedPreview bool
	}
	SignedChatCommand struct {
		Command       string
		Signer        *rsa.PublicKey
		Expiry        time.Time
		Salt          []byte
		Sender        uuid.UUID
		SignedPreview bool
		Signatures    map[string][]byte
	}
)
