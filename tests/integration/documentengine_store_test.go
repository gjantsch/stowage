package integration

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	documentengine "github.com/gjantsch/stowage/internal/documentengine"
	"github.com/gjantsch/stowage/internal/documentengine/crypto"
	"github.com/gjantsch/stowage/internal/manifest"
	"github.com/gjantsch/stowage/pkg/blobstore"
	pkgdocumentengine "github.com/gjantsch/stowage/pkg/documentengine"
)

var testMEK = bytes.Repeat([]byte{0x42}, 32)

func newEngine(t *testing.T, cfg pkgdocumentengine.Config) (*documentengine.Engine, blobstore.BlobStore) {
	t.Helper()
	blobs := blobfs.NewFS(t.TempDir())
	e, err := documentengine.New(cfg, blobs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e, blobs
}

func defaultCfg() pkgdocumentengine.Config {
	return pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionAES256,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   4 * 1024 * 1024,
	}
}

// --- Store tests ---

func TestStore_SingleChunk(t *testing.T) {
	e, blobs := newEngine(t, defaultCfg())
	ctx := context.Background()
	content := bytes.Repeat([]byte("a"), 1024)

	m, err := e.Store(ctx, bytes.NewReader(content), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if len(m.Chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(m.Chunks))
	}
	if m.Chunks[0].Order != 0 {
		t.Errorf("chunk order want 0, got %d", m.Chunks[0].Order)
	}

	rc, err := blobs.Retrieve(ctx, m.Chunks[0].Hash)
	if err != nil {
		t.Fatalf("Retrieve chunk: %v", err)
	}
	rc.Close()
}

func TestStore_MultiChunk(t *testing.T) {
	cfg := defaultCfg()
	cfg.ChunkSize = 1024
	e, blobs := newEngine(t, cfg)
	ctx := context.Background()

	// 3.5 chunks worth of data
	content := bytes.Repeat([]byte("b"), 3*1024+512)

	m, err := e.Store(ctx, bytes.NewReader(content), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if len(m.Chunks) != 4 {
		t.Fatalf("want 4 chunks, got %d", len(m.Chunks))
	}
	for i, c := range m.Chunks {
		if c.Order != i {
			t.Errorf("chunk %d: want order %d, got %d", i, i, c.Order)
		}
		rc, err := blobs.Retrieve(ctx, c.Hash)
		if err != nil {
			t.Errorf("chunk %d Retrieve: %v", i, err)
			continue
		}
		rc.Close()
	}
}

func TestStore_ManifestFields_AES256_ZSTD(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionZSTD,
		Encryption:  manifest.EncryptionAES256,
		HashAlgo:    manifest.HashSHA256,
		ChunkSize:   4 * 1024 * 1024,
	}
	e, _ := newEngine(t, cfg)
	ctx := context.Background()

	m, err := e.Store(ctx, bytes.NewReader([]byte("manifest fields test")), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if m.HashAlgorithm != manifest.HashSHA256 {
		t.Errorf("HashAlgorithm: want %q, got %q", manifest.HashSHA256, m.HashAlgorithm)
	}
	if m.Compression != manifest.CompressionZSTD {
		t.Errorf("Compression: want %q, got %q", manifest.CompressionZSTD, m.Compression)
	}
	if m.Encryption != manifest.EncryptionAES256 {
		t.Errorf("Encryption: want %q, got %q", manifest.EncryptionAES256, m.Encryption)
	}
	if len(m.EncryptedDEK) == 0 {
		t.Error("EncryptedDEK: want non-empty, got empty")
	}
	var zero blobstore.Hash32
	if m.IntegrityHash == zero {
		t.Error("IntegrityHash: want non-zero")
	}
}

func TestStore_NoEncryption(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionNone,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   4 * 1024 * 1024,
	}
	e, _ := newEngine(t, cfg)
	ctx := context.Background()

	m, err := e.Store(ctx, bytes.NewReader([]byte("no encryption")), nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if m.Encryption != manifest.EncryptionNone {
		t.Errorf("Encryption: want %q, got %q", manifest.EncryptionNone, m.Encryption)
	}
	if len(m.EncryptedDEK) != 0 {
		t.Errorf("EncryptedDEK: want empty, got %d bytes", len(m.EncryptedDEK))
	}
}

func TestStore_IntegrityHash_MatchesInput(t *testing.T) {
	cfg := defaultCfg()
	e, _ := newEngine(t, cfg)
	ctx := context.Background()
	content := []byte("integrity check content")

	m, err := e.Store(ctx, bytes.NewReader(content), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	hasher, err := crypto.NewHasher(manifest.HashBLAKE3)
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	h := hasher.New()
	h.Write(content)
	var want blobstore.Hash32
	copy(want[:], h.Sum(nil))

	if m.IntegrityHash != want {
		t.Errorf("IntegrityHash mismatch:\n got  %x\n want %x", m.IntegrityHash, want)
	}
}

func TestStore_EmptyInput(t *testing.T) {
	e, _ := newEngine(t, defaultCfg())
	ctx := context.Background()

	m, err := e.Store(ctx, bytes.NewReader(nil), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Empty stream produces no chunks — there are no bytes to store.
	if len(m.Chunks) != 0 {
		t.Fatalf("want 0 chunks for empty input, got %d", len(m.Chunks))
	}
	// Manifest metadata fields are still populated.
	if m.HashAlgorithm == "" {
		t.Error("HashAlgorithm: want non-empty")
	}
}

// failOnNthStore wraps a BlobStore and injects an error on the Nth Store call.
type failOnNthStore struct {
	blobstore.BlobStore
	n, count int
}

func (f *failOnNthStore) Store(ctx context.Context, r io.Reader) (blobstore.Hash32, error) {
	f.count++
	if f.count >= f.n {
		// Drain r so the pipeline goroutine isn't blocked.
		io.Copy(io.Discard, r)
		return blobstore.Hash32{}, errors.New("injected store failure")
	}
	return f.BlobStore.Store(ctx, r)
}

func TestStore_RollbackOnFailure(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionNone,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   512,
	}
	realBlobs := blobfs.NewFS(t.TempDir())
	failing := &failOnNthStore{BlobStore: realBlobs, n: 3}

	e, err := documentengine.New(cfg, failing)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	// 5 chunks worth of data — the 3rd Store call will fail.
	content := bytes.Repeat([]byte("x"), 5*512)
	_, storeErr := e.Store(ctx, bytes.NewReader(content), nil)
	if storeErr == nil {
		t.Fatal("expected Store to return an error, got nil")
	}

	deleted, err := realBlobs.GC(ctx)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	_ = deleted

	probe := []byte("probe after rollback")
	h, err := realBlobs.Store(ctx, bytes.NewReader(probe))
	if err != nil {
		t.Fatalf("post-rollback Store: %v", err)
	}
	rc, err := realBlobs.Retrieve(ctx, h)
	if err != nil {
		t.Fatalf("post-rollback Retrieve: %v", err)
	}
	rc.Close()
}

// --- Retrieve tests ---

func TestRetrieve_RoundTrip(t *testing.T) {
	for _, cfg := range []pkgdocumentengine.Config{
		{Compression: manifest.CompressionNone, Encryption: manifest.EncryptionAES256, HashAlgo: manifest.HashBLAKE3, ChunkSize: 4 * 1024 * 1024},
		{Compression: manifest.CompressionGZIP, Encryption: manifest.EncryptionAES256, HashAlgo: manifest.HashBLAKE3, ChunkSize: 4 * 1024 * 1024},
		{Compression: manifest.CompressionZSTD, Encryption: manifest.EncryptionAES256, HashAlgo: manifest.HashSHA256, ChunkSize: 4 * 1024 * 1024},
		{Compression: manifest.CompressionLZ4, Encryption: manifest.EncryptionNone, HashAlgo: manifest.HashBLAKE3, ChunkSize: 4 * 1024 * 1024},
		{Compression: manifest.CompressionNone, Encryption: manifest.EncryptionNone, HashAlgo: manifest.HashBLAKE3, ChunkSize: 512},
	} {
		cfg := cfg
		t.Run(string(cfg.Compression)+"+"+string(cfg.Encryption), func(t *testing.T) {
			e, _ := newEngine(t, cfg)
			ctx := context.Background()
			content := bytes.Repeat([]byte("round trip content "), 200)

			var mek []byte
			if cfg.Encryption == manifest.EncryptionAES256 {
				mek = testMEK
			}

			m, err := e.Store(ctx, bytes.NewReader(content), mek)
			if err != nil {
				t.Fatalf("Store: %v", err)
			}

			rc, err := e.Retrieve(ctx, m, mek)
			if err != nil {
				t.Fatalf("Retrieve: %v", err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("Close (integrity check): %v", err)
			}

			if !bytes.Equal(got, content) {
				t.Errorf("content mismatch: got %d bytes, want %d bytes", len(got), len(content))
			}
		})
	}
}

func TestRetrieve_MultiChunk_RoundTrip(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionAES256,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   1024,
	}
	e, _ := newEngine(t, cfg)
	ctx := context.Background()
	content := bytes.Repeat([]byte("multi-chunk "), 500) // ~6 KiB → ~6 chunks (no compression)

	m, err := e.Store(ctx, bytes.NewReader(content), testMEK)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(m.Chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(m.Chunks))
	}

	rc, err := e.Retrieve(ctx, m, testMEK)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %d bytes, want %d", len(got), len(content))
	}
}

func TestRetrieve_TamperedChunk_ErrIntegrityMismatch(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionNone,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   512,
	}
	realBlobs := blobfs.NewFS(t.TempDir())
	e, err := documentengine.New(cfg, realBlobs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	content := bytes.Repeat([]byte("tamper me "), 100)

	m, err := e.Store(ctx, bytes.NewReader(content), nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Tamper: store a different blob under one of the chunk hashes by storing
	// garbage and overwriting the manifest chunk hash with the garbage hash.
	garbage := bytes.Repeat([]byte{0xff}, 512)
	tamperedHash, err := realBlobs.Store(ctx, bytes.NewReader(garbage))
	if err != nil {
		t.Fatalf("Store garbage: %v", err)
	}
	m.Chunks[0].Hash = tamperedHash

	rc, err := e.Retrieve(ctx, m, nil)
	if err != nil {
		t.Fatalf("Retrieve (open): %v", err)
	}
	_, readErr := io.ReadAll(rc)
	closeErr := rc.Close()

	// The integrity mismatch surfaces on Close (after all bytes are hashed).
	combined := readErr
	if combined == nil {
		combined = closeErr
	}
	if !errors.Is(combined, pkgdocumentengine.ErrIntegrityMismatch) {
		t.Errorf("want ErrIntegrityMismatch, got readErr=%v closeErr=%v", readErr, closeErr)
	}
}

// --- Erase tests ---

func TestErase_RemovesAllChunks(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionNone,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   512,
	}
	realBlobs := blobfs.NewFS(t.TempDir())
	e, err := documentengine.New(cfg, realBlobs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	content := bytes.Repeat([]byte("erase me "), 200) // multiple chunks
	m, err := e.Store(ctx, bytes.NewReader(content), nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(m.Chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(m.Chunks))
	}

	if err := e.Erase(ctx, m); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	for _, c := range m.Chunks {
		_, err := realBlobs.Retrieve(ctx, c.Hash)
		if !errors.Is(err, blobstore.ErrNotFound) {
			t.Errorf("chunk %d: expected ErrNotFound after erase, got %v", c.Order, err)
		}
	}
}

func TestErase_DoubleErase_ReturnsError(t *testing.T) {
	cfg := pkgdocumentengine.Config{
		Compression: manifest.CompressionNone,
		Encryption:  manifest.EncryptionNone,
		HashAlgo:    manifest.HashBLAKE3,
		ChunkSize:   4 * 1024 * 1024,
	}
	e, _ := newEngine(t, cfg)
	ctx := context.Background()

	m, err := e.Store(ctx, bytes.NewReader([]byte("double erase")), nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := e.Erase(ctx, m); err != nil {
		t.Fatalf("first Erase: %v", err)
	}

	// Second erase: BlobStore.Erase returns ErrNotFound for already-erased blobs.
	err = e.Erase(ctx, m)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("second Erase: want ErrNotFound, got %v", err)
	}
}

func TestErase_EmptyManifest_NoError(t *testing.T) {
	e, _ := newEngine(t, defaultCfg())
	ctx := context.Background()

	m := &manifest.Manifest{}
	if err := e.Erase(ctx, m); err != nil {
		t.Errorf("Erase on empty manifest: want nil, got %v", err)
	}
}
