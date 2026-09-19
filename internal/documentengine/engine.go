package documentengine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"

	"github.com/gjantsch/stowage/internal/documentengine/chunk"
	"github.com/gjantsch/stowage/internal/documentengine/compress"
	"github.com/gjantsch/stowage/internal/documentengine/crypto"
	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/gjantsch/stowage/pkg/blobstore"
	"github.com/gjantsch/stowage/pkg/documentengine"
)

var _ documentengine.DocumentEngine = (*Engine)(nil)

type Engine struct {	compressor compress.Compressor
	encryptor  crypto.Encryptor
	hasher     crypto.Hasher
	chunker    *chunk.Chunker
	blobs      blobstore.BlobStore
}

func New(cfg documentengine.Config, blobs blobstore.BlobStore) (*Engine, error) {
	compressor, err := compress.NewCompressor(cfg.Compression)
	if err != nil {
		return nil, err
	}

	encryptor, err := crypto.NewEncryptor(cfg.Encryption)
	if err != nil {
		return nil, err
	}

	hasher, err := crypto.NewHasher(cfg.HashAlgo)
	if err != nil {
		return nil, err
	}

	return &Engine{
		compressor: compressor,
		encryptor:  encryptor,
		hasher:     hasher,
		chunker:    chunk.NewChunker(cfg.ChunkSize),
		blobs:      blobs,
	}, nil
}

func (e *Engine) Store(ctx context.Context, r io.Reader, mek []byte) (*manifest.Manifest, error) {
	integrityHasher := e.hasher.New()

	pipeReader1, pipeWriter1 := io.Pipe()
	go func() {
		wc, err := e.compressor.Compress(pipeWriter1)
		if err != nil {
			pipeWriter1.CloseWithError(err)
			return
		}
		_, err = io.Copy(wc, io.TeeReader(r, integrityHasher))
		if cerr := wc.Close(); err == nil {
			err = cerr
		}
		pipeWriter1.CloseWithError(err)
	}()

	cipherReader, encryptedDEK, err := e.encryptor.Encrypt(pipeReader1, mek)
	if err != nil {
		return nil, err
	}

	var chunks []manifest.Chunk
	var committed []blobstore.Hash32
	next := e.chunker.Chunks(cipherReader)
	for i := 0; ; i++ {
		cr, err := next()
		if err == io.EOF {
			break
		}
		if err != nil {
			e.rollback(ctx, committed)
			return nil, fmt.Errorf("read chunk %d: %w", i, err)
		}
		h, err := e.blobs.Store(ctx, cr)
		if err != nil {
			e.rollback(ctx, committed)
			return nil, fmt.Errorf("store chunk %d: %w", i, err)
		}
		committed = append(committed, h)
		chunks = append(chunks, manifest.Chunk{Order: i, Hash: h})
	}

	var integrityHash blobstore.Hash32
	copy(integrityHash[:], integrityHasher.Sum(nil))

	slog.Info("engine store: ok",
		"chunk_count", len(chunks),
		"compression", e.compressor.Algorithm(),
		"encryption", e.encryptor.Algorithm(),
		"hash_algorithm", e.hasher.Algorithm(),
		"integrity_hash", integrityHash,
	)

	return &manifest.Manifest{
		HashAlgorithm: e.hasher.Algorithm(),
		Compression:   e.compressor.Algorithm(),
		Encryption:    e.encryptor.Algorithm(),
		EncryptedDEK:  manifest.Base64Bytes(encryptedDEK),
		IntegrityHash: integrityHash,
		Chunks:        chunks,
	}, nil
}

func (e *Engine) Retrieve(ctx context.Context, m *manifest.Manifest, mek []byte) (io.ReadCloser, error) {
	hasher, err := crypto.NewHasher(m.HashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("retrieve: unsupported hash algorithm: %w", err)
	}
	encryptor, err := crypto.NewEncryptor(m.Encryption)
	if err != nil {
		return nil, fmt.Errorf("retrieve: unsupported encryption: %w", err)
	}
	compressor, err := compress.NewCompressor(m.Compression)
	if err != nil {
		return nil, fmt.Errorf("retrieve: unsupported compression: %w", err)
	}

	// Sort chunks by order so callers don't have to guarantee manifest ordering.
	ordered := make([]manifest.Chunk, len(m.Chunks))
	copy(ordered, m.Chunks)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Order < ordered[j].Order })

	// Build a single reassembled reader from all chunk readers in order.
	readers := make([]io.Reader, 0, len(ordered))
	closers := make([]io.ReadCloser, 0, len(ordered))
	for _, c := range ordered {
		rc, err := e.blobs.Retrieve(ctx, c.Hash)
		if err != nil {
			for _, cl := range closers {
				cl.Close()
			}
			return nil, fmt.Errorf("retrieve chunk %d (hash %x): %w", c.Order, c.Hash, err)
		}
		readers = append(readers, rc)
		closers = append(closers, rc)
	}
	reassembled := io.MultiReader(readers...)

	// Decrypt the reassembled ciphertext stream.
	plainStream, err := encryptor.Decrypt(reassembled, []byte(m.EncryptedDEK), mek)
	if err != nil {
		for _, cl := range closers {
			cl.Close()
		}
		return nil, fmt.Errorf("retrieve: decrypt: %w", err)
	}

	// Decompress the plaintext stream.
	decompressed, err := compressor.Decompress(plainStream)
	if err != nil {
		for _, cl := range closers {
			cl.Close()
		}
		return nil, fmt.Errorf("retrieve: decompress: %w", err)
	}

	// Wrap in an integrity-checking reader that hashes on the fly and
	// verifies against m.IntegrityHash on Close.
	integrityHasher := hasher.New()
	return &integrityReader{
		Reader:        io.TeeReader(decompressed, integrityHasher),
		decompressed:  decompressed,
		chunkClosers:  closers,
		hasher:        integrityHasher,
		expectedHash:  m.IntegrityHash,
		hashAlgorithm: m.HashAlgorithm,
		encryption:    m.Encryption,
		compression:   m.Compression,
		chunkCount:    len(ordered),
	}, nil
}

// integrityReader wraps the final plaintext stream. On Close it verifies
// the integrity hash and returns ErrIntegrityMismatch if it doesn't match.
type integrityReader struct {
	io.Reader
	decompressed  io.ReadCloser
	chunkClosers  []io.ReadCloser
	hasher        interface{ Sum([]byte) []byte }
	expectedHash  blobstore.Hash32
	hashAlgorithm manifest.HashAlgorithm
	encryption    manifest.EncryptionType
	compression   manifest.CompressionType
	chunkCount    int
}

func (ir *integrityReader) Close() error {
	if err := ir.decompressed.Close(); err != nil {
		return err
	}
	for _, cl := range ir.chunkClosers {
		cl.Close()
	}

	var got blobstore.Hash32
	copy(got[:], ir.hasher.Sum(nil))

	slog.Info("engine retrieve: ok",
		"chunk_count", ir.chunkCount,
		"compression", ir.compression,
		"encryption", ir.encryption,
		"hash_algorithm", ir.hashAlgorithm,
		"integrity_match", got == ir.expectedHash,
	)

	if got != ir.expectedHash {
		return fmt.Errorf("%w: got %x, want %x", documentengine.ErrIntegrityMismatch, got, ir.expectedHash)
	}
	return nil
}

func (e *Engine) Erase(ctx context.Context, m *manifest.Manifest) error {
	var firstErr error
	for _, c := range m.Chunks {
		if err := e.blobs.Erase(ctx, c.Hash); err != nil {
			slog.Error("engine erase: chunk erase failed", "order", c.Order, "hash", c.Hash, "err", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("erase chunk %d (hash %x): %w", c.Order, c.Hash, err)
			}
		}
	}
	if firstErr == nil {
		slog.Info("engine erase: ok", "chunk_count", len(m.Chunks))
	}
	return firstErr
}

func (e *Engine) rollback(ctx context.Context, committed []blobstore.Hash32) {
	for _, h := range committed {
		if err := e.blobs.Erase(ctx, h); err != nil {
			slog.Error("engine rollback: erase failed", "hash", h, "err", err)
		}
	}
}
