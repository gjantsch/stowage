package blobstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Hash32 is a 32-byte digest. It implements MarshalJSON/UnmarshalJSON and
// MarshalText/UnmarshalText using lowercase hex encoding for human-readable
// manifests. The text encoding is used by yaml.v3 automatically.
type Hash32 [32]byte

// MarshalJSON implements the json.Marshaler interface.
func (h Hash32) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(h[:]) + `"`), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (h *Hash32) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("Hash32: expected a JSON string")
	}
	b, err := hex.DecodeString(string(data[1 : len(data)-1]))
	if err != nil {
		return fmt.Errorf("Hash32: invalid hex: %w", err)
	}
	if len(b) != 32 {
		return fmt.Errorf("Hash32: expected 32 bytes, got %d", len(b))
	}
	copy(h[:], b)
	return nil
}

// MarshalText implements encoding.TextMarshaler (used by yaml.v3).
func (h Hash32) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(h[:])), nil
}

// UnmarshalText implements encoding.TextUnmarshaler (used by yaml.v3).
func (h *Hash32) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return fmt.Errorf("Hash32: invalid hex: %w", err)
	}
	if len(b) != 32 {
		return fmt.Errorf("Hash32: expected 32 bytes, got %d", len(b))
	}
	copy(h[:], b)
	return nil
}

// Config represents the configuration for a BlobStore.
// RootDir specifies the root directory for storing blobs.
// ChunkSize specifies the size of each chunk when storing blobs.
// Deduplicate, when true, causes Store to skip writing when slot 0 already
// exists for the computed hash — identical content is never written twice.
type Config struct {
	RootDir     string
	ChunkSize   int64
	Deduplicate bool
}

// BlobStore represents a storage system for blobs
// identified by their 32-byte hash.
type BlobStore interface {
	Store(ctx context.Context, r io.Reader) (Hash32, error)
	Retrieve(ctx context.Context, hash Hash32) (io.ReadCloser, error)
	Erase(ctx context.Context, hash Hash32) error
	GC(ctx context.Context) (int, error)
}

var ErrNotFound = errors.New("blob not found")
var ErrHashMismatch = errors.New("stored hash does not match content")
