package chunk_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/gjantsch/stowage/internal/documentengine/chunk"
)

func TestChunker(t *testing.T) {
	tests := []struct {
		name        string
		chunkSize   int64
		input       []byte
		wantChunks  int
		wantSizes   []int // expected byte count per chunk, in order
	}{
		{
			name:       "empty input",
			chunkSize:  4,
			input:      []byte{},
			wantChunks: 0,
		},
		{
			name:       "input smaller than one chunk",
			chunkSize:  8,
			input:      []byte("hello"),
			wantChunks: 1,
			wantSizes:  []int{5},
		},
		{
			name:       "input exactly one chunk",
			chunkSize:  5,
			input:      []byte("hello"),
			wantChunks: 1,
			wantSizes:  []int{5},
		},
		{
			name:       "input exactly two chunks",
			chunkSize:  4,
			input:      []byte("abcdefgh"),
			wantChunks: 2,
			wantSizes:  []int{4, 4},
		},
		{
			name:       "input spans two chunks with remainder",
			chunkSize:  4,
			input:      []byte("abcdefghi"), // 9 bytes: 4 + 4 + 1
			wantChunks: 3,
			wantSizes:  []int{4, 4, 1},
		},
		{
			name:       "invalid chunk size falls back to default",
			chunkSize:  0,
			input:      bytes.Repeat([]byte("x"), 10),
			wantChunks: 1,
			wantSizes:  []int{10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chunk.NewChunker(tt.chunkSize)
			next := c.Chunks(bytes.NewReader(tt.input))

			var got [][]byte
			for {
				r, err := next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("iterator error: %v", err)
				}
				data, err := io.ReadAll(r)
				if err != nil {
					t.Fatalf("ReadAll chunk %d: %v", len(got), err)
				}
				got = append(got, data)
			}

			if len(got) != tt.wantChunks {
				t.Fatalf("got %d chunks, want %d", len(got), tt.wantChunks)
			}
			for i, sz := range tt.wantSizes {
				if len(got[i]) != sz {
					t.Errorf("chunk %d: got %d bytes, want %d", i, len(got[i]), sz)
				}
			}

			// Reassembled content must match input.
			var reassembled []byte
			for _, d := range got {
				reassembled = append(reassembled, d...)
			}
			if !bytes.Equal(reassembled, tt.input) {
				t.Errorf("reassembled mismatch: got %d bytes, want %d", len(reassembled), len(tt.input))
			}
		})
	}
}

// TestChunker_PartialRead verifies the iterator drains unread bytes from the
// previous chunk before advancing, so callers that don't fully consume a chunk
// still get correct data from subsequent chunks.
func TestChunker_PartialRead(t *testing.T) {
	input := []byte("abcdefgh") // two 4-byte chunks
	c := chunk.NewChunker(4)
	next := c.Chunks(bytes.NewReader(input))

	// Read first chunk but consume only 2 of 4 bytes.
	r1, err := next()
	if err != nil {
		t.Fatalf("first chunk: %v", err)
	}
	partial := make([]byte, 2)
	if _, err := io.ReadFull(r1, partial); err != nil {
		t.Fatalf("partial read: %v", err)
	}

	// Second chunk must still contain "efgh", not "cdefgh".
	r2, err := next()
	if err != nil {
		t.Fatalf("second chunk: %v", err)
	}
	data, err := io.ReadAll(r2)
	if err != nil {
		t.Fatalf("ReadAll second chunk: %v", err)
	}
	if !bytes.Equal(data, []byte("efgh")) {
		t.Errorf("second chunk = %q, want %q", data, "efgh")
	}

	_, err = next()
	if err != io.EOF {
		t.Errorf("expected io.EOF after last chunk, got %v", err)
	}
}

func TestChunker_ChunkSize(t *testing.T) {
	c := chunk.NewChunker(1024)
	if c.ChunkSize() != 1024 {
		t.Errorf("ChunkSize() = %d, want 1024", c.ChunkSize())
	}
}

func TestChunker_DefaultChunkSize(t *testing.T) {
	c := chunk.NewChunker(0)
	if c.ChunkSize() != chunk.DefaultChunkSize {
		t.Errorf("ChunkSize() = %d, want %d", c.ChunkSize(), chunk.DefaultChunkSize)
	}
}
