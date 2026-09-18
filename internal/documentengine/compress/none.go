package compress

import (
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

type NoneCompressor struct{}

func NewNone() Compressor { return NoneCompressor{} }

func (NoneCompressor) Algorithm() manifest.CompressionType { return manifest.CompressionNone }

func (NoneCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return nopWriteCloser{w}, nil
}

func (NoneCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(r), nil
}

// nopWriteCloser wraps an io.Writer with a no-op Close.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
