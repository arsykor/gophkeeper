package service

import (
	"bytes"
	"context"
	"io"

	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
)

// mockUserRepo implements UserRepository for tests.
type mockUserRepo struct {
	users map[string]*postgres.User
	idSeq int
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*postgres.User)}
}

func (m *mockUserRepo) CreateUser(login, passwordHash string) (string, error) {
	if _, ok := m.users[login]; ok {
		return "", postgres.ErrLoginTaken
	}
	m.idSeq++
	id := string(rune('0' + m.idSeq))
	m.users[login] = &postgres.User{ID: id, Login: login, PasswordHash: passwordHash}
	return id, nil
}

func (m *mockUserRepo) GetUserByLogin(login string) (*postgres.User, error) {
	u, ok := m.users[login]
	if !ok {
		return nil, postgres.ErrNotFound
	}
	return u, nil
}

// mockSecretRepo implements SecretRepository for tests.
type mockSecretRepo struct {
	secrets map[string]*postgres.Secret
	idSeq   int
}

func newMockSecretRepo() *mockSecretRepo {
	return &mockSecretRepo{secrets: make(map[string]*postgres.Secret)}
}

func (m *mockSecretRepo) CreateSecret(s *postgres.Secret) (*postgres.Secret, error) {
	m.idSeq++
	s.ID = string(rune('A' + m.idSeq))
	s.Version = 1
	m.secrets[s.ID] = s
	return s, nil
}

func (m *mockSecretRepo) ListSecretsByUser(userID string) ([]*postgres.Secret, error) {
	var out []*postgres.Secret
	for _, s := range m.secrets {
		if s.UserID == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *mockSecretRepo) GetSecret(id, userID string) (*postgres.Secret, error) {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return nil, postgres.ErrNotFound
	}
	return s, nil
}

func (m *mockSecretRepo) UpdateSecret(s *postgres.Secret) (*postgres.Secret, error) {
	existing, ok := m.secrets[s.ID]
	if !ok || existing.UserID != s.UserID {
		return nil, postgres.ErrNotFound
	}
	if existing.Version != s.Version {
		return nil, postgres.ErrVersionConflict
	}
	s.Version = existing.Version + 1
	m.secrets[s.ID] = s
	return s, nil
}

func (m *mockSecretRepo) DeleteSecret(id, userID string) error {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return postgres.ErrNotFound
	}
	delete(m.secrets, id)
	return nil
}

func (m *mockSecretRepo) CreateBinarySecret(s *postgres.Secret) (*postgres.Secret, error) {
	return m.CreateSecret(s)
}

func (m *mockSecretRepo) UpdateBinarySecret(id, userID, storageKey string, fileSize int64, version int32) error {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return postgres.ErrNotFound
	}
	s.StorageKey = storageKey
	s.FileSize = fileSize
	s.Version++
	return nil
}

// mockFileStorage is a no-op FileStorage.
type mockFileStorage struct{}

func (m *mockFileStorage) Upload(_ context.Context, _ string, r io.Reader, _ int64, _ string) error {
	_, err := io.Copy(io.Discard, r)
	return err
}

func (m *mockFileStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("file-content"))), nil
}

func (m *mockFileStorage) Delete(_ context.Context, _ string) error { return nil }
