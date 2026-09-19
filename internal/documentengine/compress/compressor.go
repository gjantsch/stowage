package compress

import (
	"fmt"
	"io"

	"github.com/gjantsch/stowage/internal/manifest"
)

type Compressor interface {
	Compress(w io.Writer) (io.WriteCloser, error)
	Decompress(r io.Reader) (io.ReadCloser, error)
	Algorithm() manifest.CompressionType
}

func NewCompressor(t manifest.CompressionType) (Compressor, error) {
	switch t {
	case manifest.CompressionNone:
		return NewNone(), nil
	case manifest.CompressionGZIP:
		return NewGZIP(), nil
	case manifest.CompressionZSTD:
		return NewZSTD(), nil
	case manifest.CompressionLZ4:
		return NewLZ4(), nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %v", t)
	}
}
