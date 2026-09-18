package compress

import (
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/gjantsch/stowage/internal/manifest"
)

type ZSTDCompressor struct{}

func NewZSTD() Compressor { return ZSTDCompressor{} }

func (ZSTDCompressor) Algorithm() manifest.CompressionType { return manifest.CompressionZSTD }

func (ZSTDCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return zstd.NewWriter(w)
}

func (ZSTDCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	dec, err := zstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	return dec.IOReadCloser(), nil
}
