package tui

import (
	"encoding/json"
	"errors"
	"strings"
)

// normalizeMetadata prepares the tags field for the API.
// Empty/whitespace is sent as "" (stored as NULL in Postgres).
func normalizeMetadata(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !json.Valid([]byte(s)) {
		return "", errors.New("tags must be valid JSON — leave empty or use {} or {\"site\":\"example.com\"}")
	}
	return s, nil
}
