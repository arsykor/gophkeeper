package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataArg_Empty(t *testing.T) {
	v, err := metadataArg("")
	require.NoError(t, err)
	assert.False(t, v.Valid, "empty string should produce SQL NULL (Valid=false)")
}

func TestMetadataArg_ValidJSON(t *testing.T) {
	v, err := metadataArg(`{"site":"example.com"}`)
	require.NoError(t, err)
	assert.True(t, v.Valid)
	assert.Equal(t, `{"site":"example.com"}`, v.String)
}

func TestMetadataArg_InvalidJSON(t *testing.T) {
	_, err := metadataArg("not-json")
	assert.ErrorIs(t, err, ErrInvalidMetadata)
}
