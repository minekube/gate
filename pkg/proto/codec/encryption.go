package codec

import (
	"crypto/aes"
	"crypto/cipher"
	"io"
)

func NewDecryptReader(r io.Reader, secret []byte) (reader io.Reader, err error) {
	var block cipher.Block
	block, err = aes.NewCipher(secret)
	if err != nil {
		return
	}
	return &cipher.StreamReader{
		S: newCFB8(block, secret, true),
		R: r,
	}, nil
}

func NewEncryptWriter(w io.Writer, secret []byte) (wr io.Writer, err error) {
	var block cipher.Block
	block, err = aes.NewCipher(secret)
	if err != nil {
		return
	}
	return &cipher.StreamWriter{
		S: newCFB8(block, secret, false),
		W: w,
	}, nil
}

//
//
//
//
//
//

// AES CFB-8, version from stdlib is not working?
type cfb8 struct {
	c               cipher.Block
	blockSize       int
	iv, ivReal, tmp []byte
	de              bool
}

func newCFB8(c cipher.Block, iv []byte, decrypt bool) cipher.Stream {
	if len(iv) != 16 {
		panic("bad iv length!")
	}
	cp := make([]byte, 256)
	copy(cp, iv)
	return &cfb8{
		c:         c,
		blockSize: c.BlockSize(),
		iv:        cp[:16],
		ivReal:    cp,
		tmp:       make([]byte, 16),
		de:        decrypt,
	}
}

func (cf *cfb8) XORKeyStream(dst, src []byte) {
	for i := 0; i < len(src); i++ {
		val := src[i]
		cf.c.Encrypt(cf.tmp, cf.iv)
		val = val ^ cf.tmp[0]

		if cap(cf.iv) >= 17 {
			cf.iv = cf.iv[1:17]
		} else {
			copy(cf.ivReal, cf.iv[1:])
			cf.iv = cf.ivReal[:16]
		}

		if cf.de {
			cf.iv[15] = src[i]
		} else {
			cf.iv[15] = val
		}
		dst[i] = val
	}
}
