package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/crypto/nacl/box"
)

// WrappedKey is the JSON-encoded "wrapped key" that a backup is encrypted with.
type WrappedKey struct {
	Nonce        []byte
	CipherText   []byte
	SenderPubkey []byte
}

// wrapKey takes a public key, and an aes key to encrypt to it.
// It returns a string suitable for passing to `unwrap` (it's json)
func wrap(recipientPubkey *[32]byte, keyToWrap []byte) (wrapped []byte, err error) {
	senderPubkey, privateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, errors.Wrap(err, "Error reading random for nonce")
	}

	ciphertext := box.Seal(nil, keyToWrap, &nonce, recipientPubkey, privateKey)

	return json.Marshal(WrappedKey{Nonce: nonce[:], CipherText: ciphertext, SenderPubkey: (*senderPubkey)[:]})
}

// Unwrap takes the wrapped key from `wrap`, along with the private key.  It returns a key
// suitable to be passed to Restore()
func Unwrap(wrapped []byte, privateKey []byte) ([]byte, error) {
	wrappedKey := WrappedKey{}
	if err := json.Unmarshal(wrapped, &wrappedKey); err != nil {
		return nil, err
	}

	// box.Open takes fixed-size arrays, which don't JSON nicely.
	// So we manually check length and copy from a []byte slice to a [N]byte array.
	if len(wrappedKey.Nonce) != 24 {
		return nil, fmt.Errorf("incorrect nonce length: 24 != %d", len(wrappedKey.Nonce))
	}
	var nonce [24]byte
	copy(nonce[:], wrappedKey.Nonce)

	if len(wrappedKey.SenderPubkey) != 32 {
		return nil, fmt.Errorf("incorrect public key length: 32 != %d", len(wrappedKey.SenderPubkey))
	}
	var pubkey [32]byte
	copy(pubkey[:], wrappedKey.SenderPubkey)

	if len(privateKey) != 32 {
		return nil, fmt.Errorf("incorrect private key length: 32 != %d", len(privateKey))
	}
	var privkey [32]byte
	copy(privkey[:], privateKey)

	if len(wrappedKey.CipherText) != 32 {
		return nil, fmt.Errorf("incorrect ciphertext: 32 != %d", len(wrappedKey.SenderPubkey))
	}

	decrypted, ok := box.Open(nil, wrappedKey.CipherText, &nonce, &pubkey, &privkey)
	if !ok {
		return nil, errors.New("Decryption failed")
	}
	return decrypted, nil
}

func aesgcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// Encrypt data with a new, randomly generated key.
// Returns the key encrypted to pubkey, and the encrypted data
func encrypt(data []byte, pubkey *[32]byte) (wrappedKey []byte, ciphertext []byte, err error) {
	key := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, nil, errors.Wrap(err, "Error reading random for key")
	}
	aesgcm, err := aesgcm(key)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, errors.Wrap(err, "Error reading random for nonce")
	}

	wrapped, err := wrap(pubkey, key)
	if err != nil {
		return nil, nil, err
	}

	// Seal appends to the first parameter, so we append ciphertext to the nonce
	return wrapped, aesgcm.Seal(nonce, nonce, data, nil), nil
}

// Decrypt takes encrypted data, and the `key` returned from `unwrap`
func decrypt(data, key []byte) ([]byte, error) {
	aesgcm, err := aesgcm(key)
	if err != nil {
		return nil, err
	}

	nonceSize := aesgcm.NonceSize()
	// Nonce is prefixed to data
	return aesgcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}
