package compress_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/gjantsch/stowage/internal/documentengine/compress"
	"github.com/gjantsch/stowage/internal/manifest"
)

func TestCompressors(t *testing.T) {
	tests := []struct {
		name      string
		c         compress.Compressor
		algorithm manifest.CompressionType
		content   []byte
	}{
		{
			name:      "none/empty",
			c:         compress.NewNone(),
			algorithm: manifest.CompressionNone,
			content:   []byte{},
		},
		{
			name:      "none/short",
			c:         compress.NewNone(),
			algorithm: manifest.CompressionNone,
			content:   []byte("hello compressor"),
		},
		{
			name:      "none/repeated",
			c:         compress.NewNone(),
			algorithm: manifest.CompressionNone,
			content:   bytes.Repeat([]byte("stowage "), 1024),
		},
		{
			name:      "gzip/empty",
			c:         compress.NewGZIP(),
			algorithm: manifest.CompressionGZIP,
			content:   []byte{},
		},
		{
			name:      "gzip/short",
			c:         compress.NewGZIP(),
			algorithm: manifest.CompressionGZIP,
			content:   []byte("hello compressor"),
		},
		{
			name:      "gzip/repeated",
			c:         compress.NewGZIP(),
			algorithm: manifest.CompressionGZIP,
			content:   bytes.Repeat([]byte("stowage "), 1024),
		},
		{
			name:      "zstd/empty",
			c:         compress.NewZSTD(),
			algorithm: manifest.CompressionZSTD,
			content:   []byte{},
		},
		{
			name:      "zstd/short",
			c:         compress.NewZSTD(),
			algorithm: manifest.CompressionZSTD,
			content:   []byte("hello compressor"),
		},
		{
			name:      "zstd/repeated",
			c:         compress.NewZSTD(),
			algorithm: manifest.CompressionZSTD,
			content:   bytes.Repeat([]byte("stowage "), 1024),
		},
		{
			name:      "lz4/empty",
			c:         compress.NewLZ4(),
			algorithm: manifest.CompressionLZ4,
			content:   []byte{},
		},
		{
			name:      "lz4/short",
			c:         compress.NewLZ4(),
			algorithm: manifest.CompressionLZ4,
			content:   []byte("hello compressor"),
		},
		{
			name:      "lz4/repeated",
			c:         compress.NewLZ4(),
			algorithm: manifest.CompressionLZ4,
			content:   bytes.Repeat([]byte("stowage "), 1024),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Algorithm(); got != tt.algorithm {
				t.Fatalf("Algorithm() = %q, want %q", got, tt.algorithm)
			}

			var buf bytes.Buffer
			wc, err := tt.c.Compress(&buf)
			if err != nil {
				t.Fatalf("Compress: %v", err)
			}
			if _, err := wc.Write(tt.content); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := wc.Close(); err != nil {
				t.Fatalf("WriteCloser.Close: %v", err)
			}

			rc, err := tt.c.Decompress(&buf)
			if err != nil {
				t.Fatalf("Decompress: %v", err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("ReadCloser.Close: %v", err)
			}

			if !bytes.Equal(got, tt.content) {
				t.Errorf("round-trip mismatch: got %d bytes, want %d bytes", len(got), len(tt.content))
			}
		})
	}
}
