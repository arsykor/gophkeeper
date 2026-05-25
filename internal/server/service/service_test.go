package service

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/arsykor/gophkeeper/internal/server/auth"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
)

const testKey = "0000000000000000000000000000000000000000000000000000000000000000"

func newTestService(t *testing.T) (*Service, *mockUserRepo, *mockSecretRepo) {
	t.Helper()
	authMgr := auth.New("jwt-secret")
	enc, err := auth.NewEncryptor(testKey)
	require.NoError(t, err)
	users := newMockUserRepo()
	secrets := newMockSecretRepo()
	svc := New(users, secrets, &mockFileStorage{}, authMgr, enc)
	return svc, users, secrets
}

func TestRegister_Success(t *testing.T) {
	svc, _, _ := newTestService(t)
	token, err := svc.Register("alice", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestRegister_DuplicateLogin(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register("alice", "pass1")
	require.NoError(t, err)
	_, err = svc.Register("alice", "pass2")
	assert.ErrorIs(t, err, postgres.ErrLoginTaken)
}

func TestLogin_Success(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register("bob", "secret")
	require.NoError(t, err)

	token, err := svc.Login("bob", "secret")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register("carol", "correct")
	require.NoError(t, err)

	_, err = svc.Login("carol", "wrong")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_UnknownUser(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Login("ghost", "pass")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestCreateAndGetSecret(t *testing.T) {
	svc, _, _ := newTestService(t)
	payload := []byte(`{"login":"user","password":"pass"}`)
	sec, err := svc.CreateSecret("user-1", "My Cred", "credential", `{}`, payload)
	require.NoError(t, err)
	assert.NotEmpty(t, sec.ID)
	assert.Equal(t, "My Cred", sec.Name)

	got, err := svc.GetSecret(sec.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, payload, got.Data)
}

func TestGetSecret_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.GetSecret("nonexistent", "user-1")
	assert.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestListSecrets(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.CreateSecret("user-1", "A", "text", "", []byte("hello"))
	require.NoError(t, err)
	_, err = svc.CreateSecret("user-1", "B", "text", "", []byte("world"))
	require.NoError(t, err)
	_, err = svc.CreateSecret("user-2", "C", "text", "", []byte("other"))
	require.NoError(t, err)

	list, err := svc.ListSecrets("user-1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestUpdateSecret_Success(t *testing.T) {
	svc, _, _ := newTestService(t)
	payload := []byte(`{"content":"old"}`)
	sec, err := svc.CreateSecret("user-1", "Note", "text", "", payload)
	require.NoError(t, err)

	updated, err := svc.UpdateSecret(sec.ID, "user-1", "Note", "", []byte(`{"content":"new"}`), sec.Version)
	require.NoError(t, err)
	assert.Equal(t, int32(2), updated.Version)
}

func TestUpdateSecret_VersionConflict(t *testing.T) {
	svc, _, _ := newTestService(t)
	sec, err := svc.CreateSecret("user-1", "Note", "text", "", []byte("data"))
	require.NoError(t, err)

	_, err = svc.UpdateSecret(sec.ID, "user-1", "Note", "", []byte("new"), 99)
	assert.True(t, errors.Is(err, postgres.ErrVersionConflict))
}

func TestDeleteSecret(t *testing.T) {
	svc, _, _ := newTestService(t)
	sec, err := svc.CreateSecret("user-1", "Del", "text", "", []byte("bye"))
	require.NoError(t, err)

	err = svc.DeleteSecret(t.Context(), sec.ID, "user-1")
	require.NoError(t, err)

	_, err = svc.GetSecret(sec.ID, "user-1")
	assert.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestDeleteSecret_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	err := svc.DeleteSecret(t.Context(), "no-such-id", "user-1")
	assert.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestCreateSecret_EncryptDecryptRoundtrip(t *testing.T) {
	svc, _, _ := newTestService(t)
	payload := []byte(`{"login":"admin","password":"ILoveGolang123"}`)
	sec, err := svc.CreateSecret("user-1", "Vault", "credential", `{"site":"https://practicum.yandex.ru"}`, payload)
	require.NoError(t, err)
	// Raw stored data must differ from plaintext.
	assert.NotEqual(t, payload, sec.Data)

	// GetSecret must return the original plaintext.
	got, err := svc.GetSecret(sec.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, payload, got.Data)
}

func TestUpdateSecret_StoresEncrypted(t *testing.T) {
	svc, _, _ := newTestService(t)
	sec, err := svc.CreateSecret("user-1", "N", "text", "", []byte("v1"))
	require.NoError(t, err)

	updated, err := svc.UpdateSecret(sec.ID, "user-1", "N", "", []byte("v2"), sec.Version)
	require.NoError(t, err)
	assert.Equal(t, int32(2), updated.Version)

	got, err := svc.GetSecret(sec.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), got.Data)
}

func TestListSecrets_Empty(t *testing.T) {
	svc, _, _ := newTestService(t)
	list, err := svc.ListSecrets("user-nobody")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRegister_EmptyLogin(t *testing.T) {
	svc, _, _ := newTestService(t)
	// An empty login should still be passed to the repo (repo can reject it).
	// The important thing is service doesn't panic.
	_, err := svc.Register("", "pass")
	// mockUserRepo allows empty keys; just ensure no panic.
	assert.NoError(t, err)
}

func TestUploadFile(t *testing.T) {
	svc, _, _ := newTestService(t)
	content := []byte("binary content here")
	r := bytes.NewReader(content)

	sec, err := svc.UploadFile(t.Context(), "user-1", "file.bin", "", r, int64(len(content)))
	require.NoError(t, err)
	assert.Equal(t, "file.bin", sec.Name)
	assert.Equal(t, "binary", sec.Type)
	assert.NotEmpty(t, sec.StorageKey)
}

func TestDownloadFile(t *testing.T) {
	svc, _, _ := newTestService(t)
	content := []byte("download me")
	r := bytes.NewReader(content)

	sec, err := svc.UploadFile(t.Context(), "user-1", "dl.bin", "", r, int64(len(content)))
	require.NoError(t, err)

	_, rc, err := svc.DownloadFile(t.Context(), sec.ID, "user-1")
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	_ = data
}

func TestDownloadFile_NotBinary(t *testing.T) {
	svc, _, _ := newTestService(t)
	sec, err := svc.CreateSecret("user-1", "text", "text", "", []byte("hello"))
	require.NoError(t, err)

	_, _, err = svc.DownloadFile(t.Context(), sec.ID, "user-1")
	assert.Error(t, err)
}
