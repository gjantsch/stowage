package documentengine

import (
	"context"
	"errors"
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

var ErrIntegrityMismatch = errors.New("integrity hash mismatch")

type DocumentEngine interface {
	Store(ctx context.Context, r io.Reader, mek []byte) (*manifest.Manifest, error)
	Retrieve(ctx context.Context, m *manifest.Manifest, mek []byte) (io.ReadCloser, error)
	Erase(ctx context.Context, m *manifest.Manifest) error
}

type Config struct {
	Compression manifest.CompressionType
	Encryption  manifest.EncryptionType
	HashAlgo    manifest.HashAlgorithm
	ChunkSize   int64
}
