package service_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/arsykor/gophkeeper/internal/server/auth"
	"github.com/arsykor/gophkeeper/internal/server/service"
	"github.com/arsykor/gophkeeper/internal/server/service/mocks"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
)

const testKey = "0000000000000000000000000000000000000000000000000000000000000000"

type testDeps struct {
	users   *mocks.MockUserRepository
	secrets *mocks.MockSecretRepository
	files   *mocks.MockFileStorage
	svc     *service.Service
}

func newDeps(t *testing.T) *testDeps {
	t.Helper()
	authMgr := auth.New("jwt-secret")
	enc, err := auth.NewEncryptor(testKey)
	require.NoError(t, err)

	users := mocks.NewMockUserRepository(t)
	secrets := mocks.NewMockSecretRepository(t)
	files := mocks.NewMockFileStorage(t)

	return &testDeps{
		users:   users,
		secrets: secrets,
		files:   files,
		svc:     service.New(users, secrets, files, authMgr, enc),
	}
}

// --- Auth ---

func TestRegister_Success(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().CreateUser("alice", mock.AnythingOfType("string")).Return("uid-1", nil)

	token, err := d.svc.Register("alice", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestRegister_DuplicateLogin(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().CreateUser("alice", mock.Anything).Return("", postgres.ErrLoginTaken)

	_, err := d.svc.Register("alice", "pass")
	assert.ErrorIs(t, err, postgres.ErrLoginTaken)
}

func TestLogin_Success(t *testing.T) {
	d := newDeps(t)
	// Register to get a real bcrypt hash, then use it in the mock.
	d.users.EXPECT().CreateUser("bob", mock.AnythingOfType("string")).
		RunAndReturn(func(login, hash string) (string, error) {
			d.users.EXPECT().GetUserByLogin("bob").
				Return(&postgres.User{ID: "uid-2", Login: "bob", PasswordHash: hash}, nil)
			return "uid-2", nil
		})

	_, err := d.svc.Register("bob", "secret")
	require.NoError(t, err)

	token, err := d.svc.Login("bob", "secret")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestLogin_WrongPassword(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().CreateUser("carol", mock.AnythingOfType("string")).
		RunAndReturn(func(login, hash string) (string, error) {
			d.users.EXPECT().GetUserByLogin("carol").
				Return(&postgres.User{ID: "uid-3", Login: "carol", PasswordHash: hash}, nil)
			return "uid-3", nil
		})

	_, err := d.svc.Register("carol", "correct")
	require.NoError(t, err)

	_, err = d.svc.Login("carol", "wrong")
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestLogin_UnknownUser(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().GetUserByLogin("ghost").Return(nil, postgres.ErrNotFound)

	_, err := d.svc.Login("ghost", "pass")
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
}

// --- Secrets ---

func TestCreateAndGetSecret(t *testing.T) {
	d := newDeps(t)
	payload := []byte(`{"login":"user","password":"pass"}`)

	var stored *postgres.Secret
	d.secrets.EXPECT().CreateSecret(mock.AnythingOfType("*postgres.Secret")).
		RunAndReturn(func(s *postgres.Secret) (*postgres.Secret, error) {
			cp := *s
			cp.ID = "sec-1"
			cp.Version = 1
			stored = &cp
			return stored, nil
		})
	d.secrets.EXPECT().GetSecret("sec-1", "user-1").
		RunAndReturn(func(_, _ string) (*postgres.Secret, error) { return stored, nil })

	sec, err := d.svc.CreateSecret("user-1", "My Cred", "credential", `{}`, payload)
	require.NoError(t, err)
	assert.Equal(t, "sec-1", sec.ID)

	got, err := d.svc.GetSecret(sec.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, payload, got.Data)
}

func TestGetSecret_NotFound(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().GetSecret("no-id", "user-1").Return(nil, postgres.ErrNotFound)

	_, err := d.svc.GetSecret("no-id", "user-1")
	assert.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestListSecrets(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().ListSecretsByUser("user-1").Return([]*postgres.Secret{
		{ID: "a", UserID: "user-1"},
		{ID: "b", UserID: "user-1"},
	}, nil)

	list, err := d.svc.ListSecrets("user-1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestListSecrets_Empty(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().ListSecretsByUser("user-nobody").Return(nil, nil)

	list, err := d.svc.ListSecrets("user-nobody")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestUpdateSecret_Success(t *testing.T) {
	d := newDeps(t)

	d.secrets.EXPECT().UpdateSecret(mock.AnythingOfType("*postgres.Secret")).
		RunAndReturn(func(s *postgres.Secret) (*postgres.Secret, error) {
			cp := *s
			cp.Version = s.Version + 1
			return &cp, nil
		})

	updated, err := d.svc.UpdateSecret("sec-1", "user-1", "Note", "", []byte(`{"content":"new"}`), 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), updated.Version)
}

func TestUpdateSecret_VersionConflict(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().UpdateSecret(mock.Anything).Return(nil, postgres.ErrVersionConflict)

	_, err := d.svc.UpdateSecret("sec-1", "user-1", "Note", "", []byte("data"), 99)
	assert.ErrorIs(t, err, postgres.ErrVersionConflict)
}

func TestDeleteSecret(t *testing.T) {
	d := newDeps(t)
	sec := &postgres.Secret{ID: "sec-1", UserID: "user-1", StorageKey: ""}

	d.secrets.EXPECT().GetSecret("sec-1", "user-1").Return(sec, nil)
	d.secrets.EXPECT().DeleteSecret("sec-1", "user-1").Return(nil)

	err := d.svc.DeleteSecret(t.Context(), "sec-1", "user-1")
	require.NoError(t, err)
}

func TestDeleteSecret_WithFile(t *testing.T) {
	d := newDeps(t)
	sec := &postgres.Secret{ID: "sec-1", UserID: "user-1", StorageKey: "user-1/sec-1"}

	d.secrets.EXPECT().GetSecret("sec-1", "user-1").Return(sec, nil)
	d.files.EXPECT().Delete(mock.Anything, "user-1/sec-1").Return(nil)
	d.secrets.EXPECT().DeleteSecret("sec-1", "user-1").Return(nil)

	err := d.svc.DeleteSecret(t.Context(), "sec-1", "user-1")
	require.NoError(t, err)
}

func TestDeleteSecret_NotFound(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().GetSecret("no-id", "user-1").Return(nil, postgres.ErrNotFound)

	err := d.svc.DeleteSecret(t.Context(), "no-id", "user-1")
	assert.ErrorIs(t, err, postgres.ErrNotFound)
}

func TestCreateSecret_EncryptDecryptRoundtrip(t *testing.T) {
	d := newDeps(t)
	payload := []byte(`{"login":"admin","password":"ILoveGolang123"}`)

	var stored *postgres.Secret
	d.secrets.EXPECT().CreateSecret(mock.AnythingOfType("*postgres.Secret")).
		RunAndReturn(func(s *postgres.Secret) (*postgres.Secret, error) {
			cp := *s
			cp.ID = "sec-rt"
			cp.Version = 1
			stored = &cp
			return stored, nil
		})
	d.secrets.EXPECT().GetSecret("sec-rt", "user-1").
		RunAndReturn(func(_, _ string) (*postgres.Secret, error) { return stored, nil })

	sec, err := d.svc.CreateSecret("user-1", "Vault", "credential", `{"site":"https://practicum.yandex.ru"}`, payload)
	require.NoError(t, err)
	// Stored bytes must be encrypted (different from plaintext).
	assert.NotEqual(t, payload, stored.Data)

	got, err := d.svc.GetSecret(sec.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, payload, got.Data)
}

func TestUpdateSecret_StoresEncrypted(t *testing.T) {
	d := newDeps(t)

	var storedData []byte
	d.secrets.EXPECT().UpdateSecret(mock.AnythingOfType("*postgres.Secret")).
		RunAndReturn(func(s *postgres.Secret) (*postgres.Secret, error) {
			storedData = s.Data
			cp := *s
			cp.Version = s.Version + 1
			return &cp, nil
		})
	d.secrets.EXPECT().GetSecret("sec-1", "user-1").
		RunAndReturn(func(_, _ string) (*postgres.Secret, error) {
			return &postgres.Secret{ID: "sec-1", UserID: "user-1", Data: storedData, Version: 2}, nil
		})

	updated, err := d.svc.UpdateSecret("sec-1", "user-1", "N", "", []byte("v2"), 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), updated.Version)

	got, err := d.svc.GetSecret("sec-1", "user-1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), got.Data)
}

// --- File upload/download ---

func TestUploadFile(t *testing.T) {
	d := newDeps(t)
	content := []byte("binary content here")

	var stored *postgres.Secret
	d.secrets.EXPECT().CreateBinarySecret(mock.AnythingOfType("*postgres.Secret")).
		RunAndReturn(func(s *postgres.Secret) (*postgres.Secret, error) {
			cp := *s
			cp.ID = "bin-1"
			cp.Version = 1
			stored = &cp
			return stored, nil
		})
	d.files.EXPECT().Upload(mock.Anything, mock.AnythingOfType("string"), mock.Anything, int64(-1), "application/octet-stream").
		RunAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ string) error {
			_, err := io.Copy(io.Discard, r)
			return err
		})
	d.secrets.EXPECT().UpdateBinarySecret("bin-1", "user-1", mock.AnythingOfType("string"), int64(len(content)), int32(1)).
		Return(nil)
	d.secrets.EXPECT().GetSecret("bin-1", "user-1").
		RunAndReturn(func(_, _ string) (*postgres.Secret, error) {
			stored.StorageKey = "user-1/bin-1"
			stored.FileSize = int64(len(content))
			return stored, nil
		})

	sec, err := d.svc.UploadFile(t.Context(), "user-1", "file.bin", "", bytes.NewReader(content), -1)
	require.NoError(t, err)
	assert.Equal(t, "file.bin", sec.Name)
	assert.NotEmpty(t, sec.StorageKey)
}

func TestDownloadFile(t *testing.T) {
	d := newDeps(t)
	fileContent := []byte("download me")

	stored := &postgres.Secret{ID: "bin-2", UserID: "user-1", StorageKey: "user-1/bin-2", FileSize: int64(len(fileContent))}
	d.secrets.EXPECT().GetSecret("bin-2", "user-1").Return(stored, nil)
	d.files.EXPECT().Download(mock.Anything, "user-1/bin-2").
		Return(io.NopCloser(bytes.NewReader(fileContent)), nil)

	_, rc, err := d.svc.DownloadFile(t.Context(), "bin-2", "user-1")
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, fileContent, data)
}

func TestDownloadFile_NotBinary(t *testing.T) {
	d := newDeps(t)
	d.secrets.EXPECT().GetSecret("sec-1", "user-1").
		Return(&postgres.Secret{ID: "sec-1", UserID: "user-1", StorageKey: ""}, nil)

	_, _, err := d.svc.DownloadFile(t.Context(), "sec-1", "user-1")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errors.New("secret is not a binary file")) || err.Error() == "secret is not a binary file")
}

func TestRegister_EmptyLogin(t *testing.T) {
	d := newDeps(t)
	d.users.EXPECT().CreateUser("", mock.Anything).Return("uid-empty", nil)

	_, err := d.svc.Register("", "pass")
	assert.NoError(t, err)
}
