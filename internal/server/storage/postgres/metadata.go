package postgres

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// ErrInvalidMetadata is returned when metadata is non-empty but not valid JSON.
var ErrInvalidMetadata = errors.New("metadata must be empty or valid JSON")

// metadataArg converts a client metadata string for storage in a JSONB column.
// Empty string is stored as SQL NULL via sql.NullString.
func metadataArg(metadata string) (sql.NullString, error) {
	if metadata == "" {
		return sql.NullString{}, nil // Valid=false → SQL NULL
	}
	if !json.Valid([]byte(metadata)) {
		return sql.NullString{}, ErrInvalidMetadata
	}
	return sql.NullString{String: metadata, Valid: true}, nil
}
