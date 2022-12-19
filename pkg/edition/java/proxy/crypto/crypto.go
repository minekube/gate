package crypto

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/gate/proto"
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
	// SignatureHolder retrieves the signature holders UUID.
	// Returns null before the LoginEvent.
	SignatureHolder() uuid.UUID
	// KeyRevision retrieves the key revision.
	KeyRevision() keyrevision.Revision
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
	//
	// Does not work for 1.19.1 until the user has authenticated.
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
	revision       keyrevision.Revision
	holder         uuid.UUID

	mu struct {
		sync.Mutex
		run              bool // is true if validation has been run
		isSignatureValid bool
	}
}

var _ IdentifiedKey = (*identifiedKey)(nil)

func NewIdentifiedKey(revision keyrevision.Revision, key []byte, expiry int64, signature []byte) (IdentifiedKey, error) {
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
		expiryTemporal: time.UnixMilli(expiry).UTC(),
		revision:       revision,
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
func (i *identifiedKey) SignatureHolder() uuid.UUID {
	return i.holder
}
func (i *identifiedKey) SignatureValid() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.mu.run {
		i.mu.isSignatureValid = i.validateData(i.holder)
		i.mu.run = true
	}
	return i.mu.isSignatureValid
}
func (i *identifiedKey) VerifyDataSignature(signature []byte, toVerify ...[]byte) bool {
	return verifySignature(crypto.SHA256, i.publicKey, signature, toVerify...)
}

func (i *identifiedKey) KeyRevision() keyrevision.Revision {
	return i.revision
}

func (i *identifiedKey) validateData(verify uuid.UUID) bool {
	if i.revision == keyrevision.GenericV1 {
		pemKey := pemEncodeKey(i.publicKeyBytes, publicPemEncodeHeader)
		expires := i.expiryTemporal.UnixMilli()
		toVerify := []byte(fmt.Sprintf("%d%s", expires, pemKey))
		return verifySignature(crypto.SHA1, yggdrasilSessionPubKey, i.signature, toVerify)
	}
	if verify == uuid.Nil {
		return false
	}
	toVerify := bytes.NewBuffer(make([]byte, 0, len(i.publicKeyBytes)+3*8))
	_ = util.WriteUUID(toVerify, verify)
	_ = util.WriteInt64(toVerify, i.expiryTemporal.UnixMilli())
	_, _ = toVerify.Write(i.publicKeyBytes)
	return verifySignature(crypto.SHA1, yggdrasilSessionPubKey, i.signature, toVerify.Bytes())
}

// SetHolder sets the holder uuid for a key or returns false if incorrect.
func SetHolder(key IdentifiedKey, holder uuid.UUID) bool {
	if key == nil {
		return false
	}
	if key.SignatureHolder() == uuid.Nil {
		k, ok := key.(*identifiedKey)
		if !ok || !k.validateData(holder) {
			return false
		}
		k.holder = holder

		k.mu.Lock()
		k.mu.run = true
		k.mu.isSignatureValid = true
		k.mu.Unlock()
		return true
	}
	return key.SignatureHolder() == holder && key.SignatureValid()
}

// CanSetHolder returns true if the holder of the key can be updated.
func CanSetHolder(key IdentifiedKey) bool {
	_, ok := key.(*identifiedKey)
	return ok
}

func verifySignature(algorithm crypto.Hash, key *rsa.PublicKey, signature []byte, toVerify ...[]byte) bool {
	if len(toVerify) == 0 {
		return false
	}
	hash := algorithm.New()
	for _, b := range toVerify {
		_, _ = hash.Write(b)
	}
	hashed := hash.Sum(nil)
	err := rsa.VerifyPKCS1v15(key, algorithm, hashed, signature)
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

const (
	publicPemEncodeHeader = "RSA PUBLIC KEY"
)

func pemEncodeKey(key []byte, header string) string {
	w := new(strings.Builder)
	enc := base64.NewEncoder(base64.StdEncoding, newLineSplitterWriter(76, []byte("\n"), w))
	_, _ = io.Copy(enc, bytes.NewReader(key))
	const format = "-----BEGIN %s-----\n%s\n-----END %s-----\n"
	return fmt.Sprintf(format, header, w.String(), header)
}

type SignaturePair struct {
	Signer    uuid.UUID
	Signature []byte
}

func (p *SignaturePair) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.Signer, err = util.ReadUUID(rd)
	if err != nil {
		return err
	}
	p.Signature, err = util.ReadBytes(rd)
	return err
}

func (p *SignaturePair) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteUUID(wr, p.Signer)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, p.Signature)
}

func ReadPlayerKey(protocol proto.Protocol, rd io.Reader) (IdentifiedKey, error) {
	expiry, err := util.ReadInt64(rd)
	if err != nil {
		return nil, err
	}
	key, err := util.ReadBytes(rd)
	if err != nil {
		return nil, err
	}
	signature, err := util.ReadBytesLen(rd, 4096)
	if err != nil {
		return nil, err
	}
	revision := keyrevision.LinkedV2
	if protocol == version.Minecraft_1_19.Protocol {
		revision = keyrevision.GenericV1
	}
	return NewIdentifiedKey(revision, key, expiry, signature)
}

func WritePlayerKey(wr io.Writer, playerKey IdentifiedKey) error {
	err := util.WriteInt64(wr, playerKey.ExpiryTemporal().UnixMilli())
	if err != nil {
		return err
	}
	err = util.WriteBytes(wr, playerKey.SignedPublicKeyBytes())
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, playerKey.Signature())
}
