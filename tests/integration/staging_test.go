package integration

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/internal/blobstore/staging"
)

type errorReader struct{}

func (e errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "stowage-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestStage_StoresFile(t *testing.T) {
	root := tempDir(t)
	stager := staging.NewStager(root, root, false)
	content := []byte("hello stowage")

	hash, err := stager.Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}

	s := shard.NewShard(hash, root)
	path := s.FilePathAt(0)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s, got error: %v", path, err)
	}

	entries, _ := filepath.Glob(filepath.Join(root, "_tmp_*"))
	if len(entries) > 0 {
		t.Errorf("found leftover temp files: %v", entries)
	}
}

func TestStage_DeterministicHash(t *testing.T) {
	root := tempDir(t)
	content := []byte("deterministic")

	hash1, _ := staging.NewStager(root, root, false).Stage(bytes.NewReader(content), sha256.New())
	hash2, _ := staging.NewStager(root, root, false).Stage(bytes.NewReader(content), sha256.New())

	if hash1 != hash2 {
		t.Errorf("same content produced different hashes: %x vs %x", hash1, hash2)
	}
}

func TestStage_DuplicateStore_WritesNewSlot(t *testing.T) {
	root := tempDir(t)
	content := []byte("duplicate content")

	hash, err := staging.NewStager(root, root, false).Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("first Stage: %v", err)
	}

	hash2, err := staging.NewStager(root, root, false).Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("second Stage: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hashes differ: %x vs %x", hash, hash2)
	}

	s := shard.NewShard(hash, root)
	if _, err := os.Stat(s.FilePathAt(0)); err != nil {
		t.Errorf("slot 0 missing: %v", err)
	}
	if _, err := os.Stat(s.FilePathAt(1)); err != nil {
		t.Errorf("slot 1 missing: %v", err)
	}
}

func TestStage_Dedup_SkipsWrite(t *testing.T) {
	root := tempDir(t)
	content := []byte("dedup content")

	hash, err := staging.NewStager(root, root, true).Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("first Stage: %v", err)
	}

	hash2, err := staging.NewStager(root, root, true).Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("second Stage: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hashes differ: %x vs %x", hash, hash2)
	}

	s := shard.NewShard(hash, root)
	if _, err := os.Stat(s.FilePathAt(0)); err != nil {
		t.Errorf("slot 0 missing: %v", err)
	}
	if _, err := os.Stat(s.FilePathAt(1)); err == nil {
		t.Errorf("slot 1 should not exist in dedup mode")
	}
}

func TestStage_NoLeftoverOnFailure(t *testing.T) {
	root := tempDir(t)
	stager := staging.NewStager(root, root, false)

	_, err := stager.Stage(errorReader{}, sha256.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	entries, _ := filepath.Glob(filepath.Join(root, "_tmp_*"))
	if len(entries) > 0 {
		t.Errorf("found leftover temp files after failure: %v", entries)
	}
}
