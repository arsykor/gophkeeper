package grpc_test

import (
	"context"
	"io"

	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
)

type mockUsers struct {
	users map[string]*postgres.User
	seq   int
}

func newMockUsers() *mockUsers {
	return &mockUsers{users: make(map[string]*postgres.User)}
}

func (m *mockUsers) CreateUser(login, passwordHash string) (string, error) {
	if _, ok := m.users[login]; ok {
		return "", postgres.ErrLoginTaken
	}
	m.seq++
	id := string(rune('a' + m.seq))
	m.users[login] = &postgres.User{ID: id, Login: login, PasswordHash: passwordHash}
	return id, nil
}

func (m *mockUsers) GetUserByLogin(login string) (*postgres.User, error) {
	u, ok := m.users[login]
	if !ok {
		return nil, postgres.ErrNotFound
	}
	return u, nil
}

type mockSecrets struct {
	secrets map[string]*postgres.Secret
	seq     int
}

func newMockSecrets() *mockSecrets {
	return &mockSecrets{secrets: make(map[string]*postgres.Secret)}
}

func (m *mockSecrets) CreateSecret(s *postgres.Secret) (*postgres.Secret, error) {
	m.seq++
	s.ID = string(rune('A' + m.seq))
	s.Version = 1
	cp := *s
	m.secrets[s.ID] = &cp
	return &cp, nil
}

func (m *mockSecrets) ListSecretsByUser(userID string) ([]*postgres.Secret, error) {
	var out []*postgres.Secret
	for _, s := range m.secrets {
		if s.UserID == userID {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockSecrets) GetSecret(id, userID string) (*postgres.Secret, error) {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return nil, postgres.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *mockSecrets) UpdateSecret(s *postgres.Secret) (*postgres.Secret, error) {
	existing, ok := m.secrets[s.ID]
	if !ok || existing.UserID != s.UserID {
		return nil, postgres.ErrNotFound
	}
	if existing.Version != s.Version {
		return nil, postgres.ErrVersionConflict
	}
	s.Version = existing.Version + 1
	cp := *s
	m.secrets[s.ID] = &cp
	return &cp, nil
}

func (m *mockSecrets) DeleteSecret(id, userID string) error {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return postgres.ErrNotFound
	}
	delete(m.secrets, id)
	return nil
}

func (m *mockSecrets) CreateBinarySecret(s *postgres.Secret) (*postgres.Secret, error) {
	return m.CreateSecret(s)
}

func (m *mockSecrets) UpdateBinarySecret(id, userID, storageKey string, fileSize int64, _ int32) error {
	s, ok := m.secrets[id]
	if !ok || s.UserID != userID {
		return postgres.ErrNotFound
	}
	s.StorageKey = storageKey
	s.FileSize = fileSize
	s.Version++
	return nil
}

type noopFiles struct{}

func (n *noopFiles) Upload(_ context.Context, _ string, r io.Reader, _ int64, _ string) error {
	// Drain the reader to unblock the pipe writer goroutine.
	_, err := io.Copy(io.Discard, r)
	return err
}

func (n *noopFiles) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(&emptyReader{}), nil
}

func (n *noopFiles) Delete(_ context.Context, _ string) error { return nil }

type emptyReader struct{}

func (e *emptyReader) Read(_ []byte) (int, error) { return 0, io.EOF }
