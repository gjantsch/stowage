package compress

import (
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

type Compressor interface {
	Compress(w io.Writer) (io.WriteCloser, error)
	Decompress(r io.Reader) (io.ReadCloser, error)
	Algorithm() manifest.CompressionType
}
