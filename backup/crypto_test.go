package backup

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

// TestEncryptDecrypt takes a random buffer, encrypts, then unwraps and decrypts.
func TestEncryptDecrypt(t *testing.T) {
	// Make a random buffer of data to test with:
	testData := make([]byte, 1234)
	copyData := make([]byte, 1234)
	_, err := io.ReadFull(rand.Reader, testData)
	require.NoError(t, err)
	copy(copyData, testData)

	pubkey, privkey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	key, ciphertext, err := encrypt(testData, pubkey)
	assert.NoError(t, err)

	// from crypto/cipher/gcm.go
	gcmStandardNonceSize := 12
	gcmTagSize := 16

	// The ciphertext should be longer than the plaintext
	assert.Equal(t, len(testData)+gcmStandardNonceSize+gcmTagSize, len(ciphertext))

	// We can't really make any other assertions about the ciphertext
	// But make sure the ciphertext doesn't literally contain the plaintext
	assert.False(t, bytes.Contains(ciphertext, copyData))

	unwrappedKey, err := Unwrap(key, privkey[:])
	assert.NoError(t, err)

	plaintext, err := decrypt(ciphertext, unwrappedKey)
	assert.NoError(t, err)

	// Verify the plaintext roundtripped
	assert.Equal(t, copyData, plaintext)

	// Verify the testData wasn't modified during encryption
	assert.Equal(t, copyData, testData)
}
