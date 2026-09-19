package chunk

import (
	"bytes"
	"io"
)

const DefaultChunkSize int64 = 4 * 1024 * 1024 // 4 MiB

type Chunker struct {
	chunkSize int64
}

func NewChunker(chunkSize int64) *Chunker {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	return &Chunker{chunkSize: chunkSize}
}

func (c *Chunker) ChunkSize() int64 { return c.chunkSize }

// Chunks returns an iterator. Each call returns the next chunk as an io.Reader
// plus nil, or (nil, io.EOF) when the stream is exhausted, or (nil, err) on
// error. The caller must either fully drain each returned reader or ignore it —
// partial reads are safe because the iterator discards any unread bytes before
// advancing to the next chunk.
func (c *Chunker) Chunks(r io.Reader) func() (io.Reader, error) {
	var current io.Reader

	return func() (io.Reader, error) {
		// Drain whatever the caller left unread from the previous chunk so the
		// underlying reader is positioned at the start of the next chunk.
		if current != nil {
			if _, err := io.Copy(io.Discard, current); err != nil {
				return nil, err
			}
		}

		// Probe one byte to distinguish "stream exhausted" from "limit reached".
		// io.LimitReader alone cannot tell the difference.
		buf := make([]byte, 1)
		n, err := r.Read(buf)
		if n == 0 {
			if err == io.EOF {
				return nil, io.EOF
			}
			return nil, err
		}

		current = io.MultiReader(
			bytes.NewReader(buf[:n]),
			io.LimitReader(r, c.chunkSize-1),
		)
		return current, nil
	}
}
