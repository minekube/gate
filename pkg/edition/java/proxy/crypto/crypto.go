package crypto

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
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
	SignedPublicKeyBytes() []byte
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
	publicKeyBytes []byte
	publicKey      *rsa.PublicKey
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
	pk, err := x509.ParsePKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("error parse public key: %w", err)
	}
	rsaKey, ok := pk.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("expected rsa public key, but got %T", pk)
	}
	return &identifiedKey{
		publicKeyBytes: key,
		publicKey:      rsaKey,
		signature:      signature,
		expiryTemporal: time.UnixMilli(expiry),
	}, nil
}

//go:embed yggdrasil_session_pubkey.der
var yggdrasilSessionPubKeyDER []byte

var yggdrasilSessionPubKey *rsa.PublicKey

func parseYggdrasilSessionPubKey() *rsa.PublicKey {
	pk, err := x509.ParsePKIXPublicKey(yggdrasilSessionPubKeyDER)
	if err != nil {
		panic(err)
	}
	return pk.(*rsa.PublicKey)
}

func init() { yggdrasilSessionPubKey = parseYggdrasilSessionPubKey() }

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
	return i.publicKey
}

func (i *identifiedKey) SignedPublicKeyBytes() []byte {
	return i.publicKeyBytes
}

func (i *identifiedKey) SignatureValid() bool {
	i.once.Do(func() {
		pemKey := pemEncodeKey(i.publicKey)
		expires := i.expiryTemporal.UnixMilli()
		toVerify := []byte(fmt.Sprintf("%d%s", expires, pemKey))
		//exp := new(bytes.Buffer)
		//_ = util.WriteBytes(exp, expires)

		//data := make([]byte, len(pemKey), len(pemKey)+len(toVerify))
		//copy(data, pemKey)
		//data = append(data, toVerify...)
		//digest := sha1.Sum(data)
		//i.once.err = rsa.VerifyPKCS1v15(yggdrasilSessionPubKey, crypto.SHA1, digest[:], i.Signature())
		//if err != nil {
		//	return
		//}
		//i.once.isSignatureValid = err == nil

		i.once.isSignatureValid = verifySignature(
			crypto.SHA1, yggdrasilSessionPubKey, i.signature, toVerify)
	})
	return i.once.isSignatureValid
}

func (i *identifiedKey) VerifyDataSignature(signature []byte, toVerify ...[]byte) bool {
	return verifySignature(crypto.SHA256, i.publicKey, signature, toVerify...)
}

func verifySignature(algorithm crypto.Hash, key *rsa.PublicKey, signature []byte, toVerify ...[]byte) bool {
	if len(toVerify) == 0 {
		return false
	}
	hash := algorithm.New()
	for _, b := range toVerify {
		_, _ = hash.Write(b)
	}
	err := rsa.VerifyPKCS1v15(key, algorithm, hash.Sum(nil), signature)
	return err == nil
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

func pemEncodeKey(publicKey *rsa.PublicKey) string {
	encoded := x509.MarshalPKCS1PublicKey(publicKey)
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: encoded,
	}))
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
