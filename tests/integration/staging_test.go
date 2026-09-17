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
	stager := staging.NewStager(root, root)
	content := []byte("hello stowage")

	hash, err := stager.Stage(bytes.NewReader(content), sha256.New())
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}

	// verify file exists at the sharded path
	s := shard.NewShard(hash, root)
	if _, err := os.Stat(s.FilePath()); err != nil {
		t.Errorf("expected file at %s, got error: %v", s.FilePath(), err)
	}

	// after Stage succeeds, no _tmp_ files should remain
	entries, _ := filepath.Glob(filepath.Join(root, "_tmp_*"))
	if len(entries) > 0 {
		t.Errorf("found leftover temp files: %v", entries)
	}
}

func TestStage_DeterministicHash(t *testing.T) {
	root := tempDir(t)
	content := []byte("deterministic")

	hash1, _ := staging.NewStager(root, root).Stage(bytes.NewReader(content), sha256.New())
	hash2, _ := staging.NewStager(root, root).Stage(bytes.NewReader(content), sha256.New())

	if hash1 != hash2 {
		t.Errorf("same content produced different hashes: %x vs %x", hash1, hash2)
	}
}

func TestStage_NoLeftoverOnFailure(t *testing.T) {
	root := tempDir(t)
	stager := staging.NewStager(root, root)

	_, err := stager.Stage(errorReader{}, sha256.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	entries, _ := filepath.Glob(filepath.Join(root, "_tmp_*"))
	if len(entries) > 0 {
		t.Errorf("found leftover temp files after failure: %v", entries)
	}
}
