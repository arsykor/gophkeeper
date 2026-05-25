// Package service implements the business logic for GophKeeper.
package service

import (
	"context"
	"errors"
	"io"

	"github.com/arsykor/gophkeeper/internal/server/auth"
	"github.com/arsykor/gophkeeper/internal/server/storage/minio"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
)

// ErrInvalidCredentials is returned when login/password do not match.
var ErrInvalidCredentials = errors.New("invalid credentials")

// UserRepository is the storage interface for user operations.
type UserRepository interface {
	CreateUser(login, passwordHash string) (string, error)
	GetUserByLogin(login string) (*postgres.User, error)
}

// SecretRepository is the storage interface for secret operations.
type SecretRepository interface {
	CreateSecret(s *postgres.Secret) (*postgres.Secret, error)
	ListSecretsByUser(userID string) ([]*postgres.Secret, error)
	GetSecret(id, userID string) (*postgres.Secret, error)
	UpdateSecret(s *postgres.Secret) (*postgres.Secret, error)
	DeleteSecret(id, userID string) error
	CreateBinarySecret(s *postgres.Secret) (*postgres.Secret, error)
	UpdateBinarySecret(id, userID, storageKey string, fileSize int64, version int32) error
}

// FileStorage is the interface for binary file operations in object storage.
type FileStorage interface {
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Service bundles all business logic dependencies.
type Service struct {
	users     UserRepository
	secrets   SecretRepository
	files     FileStorage
	authMgr   *auth.Manager
	encryptor *auth.Encryptor
}

// New creates a Service.
func New(users UserRepository, secrets SecretRepository, files FileStorage, authMgr *auth.Manager, enc *auth.Encryptor) *Service {
	return &Service{
		users:     users,
		secrets:   secrets,
		files:     files,
		authMgr:   authMgr,
		encryptor: enc,
	}
}

// Register creates a new user account and returns a JWT.
func (s *Service) Register(login, password string) (string, error) {
	hash, err := s.authMgr.HashPassword(password)
	if err != nil {
		return "", err
	}
	userID, err := s.users.CreateUser(login, hash)
	if err != nil {
		return "", err
	}
	return s.authMgr.IssueToken(userID)
}

// Login authenticates the user and returns a JWT.
func (s *Service) Login(login, password string) (string, error) {
	user, err := s.users.GetUserByLogin(login)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if err := s.authMgr.CheckPassword(password, user.PasswordHash); err != nil {
		return "", ErrInvalidCredentials
	}
	return s.authMgr.IssueToken(user.ID)
}

// ListSecrets returns metadata for all secrets owned by userID.
func (s *Service) ListSecrets(userID string) ([]*postgres.Secret, error) {
	return s.secrets.ListSecretsByUser(userID)
}

// GetSecret returns the full decrypted secret for userID.
func (s *Service) GetSecret(id, userID string) (*postgres.Secret, error) {
	sec, err := s.secrets.GetSecret(id, userID)
	if err != nil {
		return nil, err
	}
	if len(sec.Data) > 0 {
		plain, err := s.encryptor.Decrypt(sec.Data)
		if err != nil {
			return nil, err
		}
		sec.Data = plain
	}
	return sec, nil
}

// CreateSecret encrypts the payload and stores a new secret.
func (s *Service) CreateSecret(userID, name, secretType, metadata string, payload []byte) (*postgres.Secret, error) {
	encrypted, err := s.encryptor.Encrypt(payload)
	if err != nil {
		return nil, err
	}
	return s.secrets.CreateSecret(&postgres.Secret{
		UserID:   userID,
		Type:     secretType,
		Name:     name,
		Metadata: metadata,
		Data:     encrypted,
	})
}

// UpdateSecret encrypts the payload and updates the secret using optimistic locking.
func (s *Service) UpdateSecret(id, userID, name, metadata string, payload []byte, version int32) (*postgres.Secret, error) {
	encrypted, err := s.encryptor.Encrypt(payload)
	if err != nil {
		return nil, err
	}
	return s.secrets.UpdateSecret(&postgres.Secret{
		ID:       id,
		UserID:   userID,
		Name:     name,
		Metadata: metadata,
		Data:     encrypted,
		Version:  version,
	})
}

// DeleteSecret removes a secret and its associated object storage file if any.
func (s *Service) DeleteSecret(ctx context.Context, id, userID string) error {
	sec, err := s.secrets.GetSecret(id, userID)
	if err != nil {
		return err
	}
	if sec.StorageKey != "" {
		_ = s.files.Delete(ctx, sec.StorageKey)
	}
	return s.secrets.DeleteSecret(id, userID)
}

// UploadFile streams binary data to MinIO and records metadata in Postgres.
func (s *Service) UploadFile(ctx context.Context, userID, name, metadata string, r io.Reader, size int64) (*postgres.Secret, error) {
	// Insert placeholder to get the ID first.
	sec, err := s.secrets.CreateBinarySecret(&postgres.Secret{
		UserID:   userID,
		Type:     "binary",
		Name:     name,
		Metadata: metadata,
	})
	if err != nil {
		return nil, err
	}
	key := minio.ObjectKey(userID, sec.ID)
	if err := s.files.Upload(ctx, key, r, size, "application/octet-stream"); err != nil {
		// Rollback
		_ = s.secrets.DeleteSecret(sec.ID, userID)
		return nil, err
	}
	if err := s.secrets.UpdateBinarySecret(sec.ID, userID, key, size, sec.Version); err != nil {
		return nil, err
	}
	return s.secrets.GetSecret(sec.ID, userID)
}

// DownloadFile fetches binary data from MinIO for the given secret.
// Returns the secret metadata and a reader the caller must close.
func (s *Service) DownloadFile(ctx context.Context, id, userID string) (*postgres.Secret, io.ReadCloser, error) {
	sec, err := s.secrets.GetSecret(id, userID)
	if err != nil {
		return nil, nil, err
	}
	if sec.StorageKey == "" {
		return nil, nil, errors.New("secret is not a binary file")
	}
	rc, err := s.files.Download(ctx, sec.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return sec, rc, nil
}
