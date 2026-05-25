package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndCheck(t *testing.T) {
	m := New("secret")
	hash, err := m.HashPassword("mypassword")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	assert.NoError(t, m.CheckPassword("mypassword", hash))
	assert.Error(t, m.CheckPassword("wrongpassword", hash))
}

func TestIssueAndValidateToken(t *testing.T) {
	m := New("supersecret")
	token, err := m.IssueToken("user-123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := m.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
}

func TestValidateToken_Invalid(t *testing.T) {
	m := New("secret")
	_, err := m.ValidateToken("not-a-token")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	m1 := New("secret1")
	m2 := New("secret2")
	token, err := m1.IssueToken("user-1")
	require.NoError(t, err)
	_, err = m2.ValidateToken(token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}
