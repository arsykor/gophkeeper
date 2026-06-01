package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validKey = "0000000000000000000000000000000000000000000000000000000000000000"

func TestEncryptDecrypt(t *testing.T) {
	enc, err := NewEncryptor(validKey)
	require.NoError(t, err)

	plaintext := []byte("hello, world!")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ciphertext)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptor_InvalidKey(t *testing.T) {
	_, err := NewEncryptor("tooshort")
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	enc, _ := NewEncryptor(validKey)
	ct, _ := enc.Encrypt([]byte("data"))
	ct[len(ct)-1] ^= 0xff // flip last byte
	_, err := enc.Decrypt(ct)
	assert.Error(t, err)
}
