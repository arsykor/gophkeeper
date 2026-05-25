package postgres

import (
	"database/sql"
	"errors"
)

// CreateSecret inserts a new secret row and returns it with generated fields filled.
func (db *DB) CreateSecret(s *Secret) (*Secret, error) {
	meta, err := metadataArg(s.Metadata)
	if err != nil {
		return nil, err
	}
	row := db.conn.QueryRow(`
		INSERT INTO secrets (user_id, type, name, metadata, data, storage_key, file_size)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, version, created_at, updated_at`,
		s.UserID, s.Type, s.Name, meta, s.Data, s.StorageKey, s.FileSize,
	)
	if err := row.Scan(&s.ID, &s.Version, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	return s, nil
}

// ListSecretsByUser returns metadata for all secrets owned by userID (no Data field).
func (db *DB) ListSecretsByUser(userID string) ([]*Secret, error) {
	rows, err := db.conn.Query(`
		SELECT id, user_id, type, name, COALESCE(metadata::text, ''), storage_key, file_size, version, created_at, updated_at
		FROM secrets WHERE user_id = $1 ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []*Secret
	for rows.Next() {
		s := &Secret{}
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.Type, &s.Name, &s.Metadata,
			&s.StorageKey, &s.FileSize, &s.Version, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// GetSecret returns the full secret row including Data.
func (db *DB) GetSecret(id, userID string) (*Secret, error) {
	s := &Secret{}
	var storageKey sql.NullString
	err := db.conn.QueryRow(`
		SELECT id, user_id, type, name, COALESCE(metadata::text, ''), data, storage_key, file_size, version, created_at, updated_at
		FROM secrets WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(
		&s.ID, &s.UserID, &s.Type, &s.Name, &s.Metadata,
		&s.Data, &storageKey, &s.FileSize, &s.Version, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if storageKey.Valid {
		s.StorageKey = storageKey.String
	}
	return s, nil
}

// UpdateSecret replaces payload using optimistic locking on version.
// Returns ErrVersionConflict when no row matched id+userID+version.
func (db *DB) UpdateSecret(s *Secret) (*Secret, error) {
	meta, err := metadataArg(s.Metadata)
	if err != nil {
		return nil, err
	}
	result, err := db.conn.Exec(`
		UPDATE secrets
		SET name = $1, metadata = $2, data = $3, version = version + 1, updated_at = now()
		WHERE id = $4 AND user_id = $5 AND version = $6`,
		s.Name, meta, s.Data, s.ID, s.UserID, s.Version,
	)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrVersionConflict
	}
	return db.GetSecret(s.ID, s.UserID)
}

// DeleteSecret removes the secret owned by userID. Returns ErrNotFound if absent.
func (db *DB) DeleteSecret(id, userID string) error {
	result, err := db.conn.Exec(
		`DELETE FROM secrets WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateBinarySecret inserts a secret whose payload is in MinIO (no data column).
func (db *DB) CreateBinarySecret(s *Secret) (*Secret, error) {
	meta, err := metadataArg(s.Metadata)
	if err != nil {
		return nil, err
	}
	row := db.conn.QueryRow(`
		INSERT INTO secrets (user_id, type, name, metadata, storage_key, file_size)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, version, created_at, updated_at`,
		s.UserID, s.Type, s.Name, meta, s.StorageKey, s.FileSize,
	)
	if err := row.Scan(&s.ID, &s.Version, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateBinarySecret updates storage_key and file_size for a binary secret.
func (db *DB) UpdateBinarySecret(id, userID, storageKey string, fileSize int64, version int32) error {
	result, err := db.conn.Exec(`
		UPDATE secrets
		SET storage_key = $1, file_size = $2, version = version + 1, updated_at = now()
		WHERE id = $3 AND user_id = $4 AND version = $5`,
		storageKey, fileSize, id, userID, version,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrVersionConflict
	}
	return nil
}
