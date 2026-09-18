package compress

import (
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/pierrec/lz4/v4"
)

type LZ4Compressor struct{}

func NewLZ4() Compressor { return LZ4Compressor{} }

func (LZ4Compressor) Algorithm() manifest.CompressionType { return manifest.CompressionLZ4 }

func (LZ4Compressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return lz4.NewWriter(w), nil
}

func (LZ4Compressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(lz4.NewReader(r)), nil
}
