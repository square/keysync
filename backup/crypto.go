package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/pkg/errors"
)

func aesgcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

func encrypt(data, key []byte) ([]byte, error) {
	aesgcm, err := aesgcm(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "Error reading random for nonce")
	}

	// Seal appends to the first parameter, so we append ciphertext to the nonce
	return aesgcm.Seal(nonce, nonce, data, nil), nil
}

func decrypt(data, key []byte) ([]byte, error) {
	aesgcm, err := aesgcm(key)
	if err != nil {
		return nil, err
	}

	nonceSize := aesgcm.NonceSize()
	// Nonce is prefixed to data
	return aesgcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}
