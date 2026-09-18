package compress

import (
	"compress/gzip"
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

type GZIPCompressor struct{}

func NewGZIP() Compressor { return &GZIPCompressor{} }

func (c *GZIPCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	gz := gzip.NewWriter(w)
	return gz, nil
}

func (c *GZIPCompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return gz, nil
}

func (c *GZIPCompressor) Algorithm() manifest.CompressionType {
	return manifest.CompressionGZIP
}
