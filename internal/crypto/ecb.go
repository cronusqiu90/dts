package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
)

var (
	encrypter cipher.BlockMode
	decrypter cipher.BlockMode
)

func InitKey(key string) {
	block, _ := aes.NewCipher([]byte(key))
	encrypter = newECBEncrypter(block)
	decrypter = newECBDecrypter(block)
}

func Encrypt(plaintext []byte) ([]byte, error) {
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	paddedPlaintext := append(plaintext, bytes.Repeat([]byte{byte(padding)}, padding)...)
	ciphertext := make([]byte, len(paddedPlaintext))
	encrypter.CryptBlocks(ciphertext, paddedPlaintext)
	return ciphertext, nil
}

func Decrypt(ciphertext []byte) ([]byte, error) {
	decryptedText := make([]byte, len(ciphertext))
	decrypter.CryptBlocks(decryptedText, ciphertext)
	padding := decryptedText[len(decryptedText)-1]
	decryptedText = decryptedText[:len(decryptedText)-int(padding)]
	return decryptedText, nil
}

type ecb struct {
	b         cipher.Block
	blockSize int
}

func newECB(b cipher.Block) *ecb {
	return &ecb{
		b:         b,
		blockSize: b.BlockSize(),
	}
}

type ecbEncrypter ecb

func newECBEncrypter(b cipher.Block) cipher.BlockMode {
	return (*ecbEncrypter)(newECB(b))
}

func (x *ecbEncrypter) BlockSize() int {
	return x.blockSize
}

func (x *ecbEncrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.blockSize != 0 {
		panic("crypto/cipher: input not full blocks")
	}
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	for len(src) > 0 {
		x.b.Encrypt(dst, src[:x.blockSize])
		src = src[x.blockSize:]
		dst = dst[x.blockSize:]
	}
}

type ecbDecrypter ecb

func newECBDecrypter(b cipher.Block) cipher.BlockMode {
	return (*ecbDecrypter)(newECB(b))
}

func (x *ecbDecrypter) BlockSize() int {
	return x.blockSize
}

func (x *ecbDecrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.blockSize != 0 {
		panic("crypto/cipher: input not full blocks")
	}
	if len(dst) < len(src) {
		panic("crypto/cipher: output smaller than input")
	}
	for len(src) > 0 {
		x.b.Decrypt(dst, src[:x.blockSize])
		src = src[x.blockSize:]
		dst = dst[x.blockSize:]
	}
}
