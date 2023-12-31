package codec

import (
	"crypto/aes"
	"crypto/cipher"
	cfb8 "github.com/Tnze/go-mc/net/CFB8"
	"io"
)

func NewDecryptReader(r io.Reader, secret []byte) (reader io.Reader, err error) {
	cfb, err := newCFB8FromSecret(secret, true)
	if err != nil {
		return nil, err
	}
	return &cipher.StreamReader{S: cfb, R: r}, nil
}

func NewEncryptWriter(w io.Writer, secret []byte) (wr io.Writer, err error) {
	cfb, err := newCFB8FromSecret(secret, false)
	if err != nil {
		return nil, err
	}
	return &cipher.StreamWriter{S: cfb, W: w}, nil
}

func newCFB8FromSecret(secret []byte, decrypt bool) (cipher.Stream, error) {
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	return newCFB8(block, secret, decrypt), nil
}

func newCFB8(c cipher.Block, iv []byte, decrypt bool) cipher.Stream {
	if decrypt {
		return cfb8.NewCFB8Decrypt(c, iv)
	} else {
		return cfb8.NewCFB8Encrypt(c, iv)
	}
}
