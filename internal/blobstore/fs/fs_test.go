package fs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blobfs "github.com/gjantsch/stowage/internal/blobstore/fs"
	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/pkg/blobstore"
)

func TestRetrieve_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	content := []byte("hello stowage retrieve")
	hash, err := store.Store(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	rc, err := store.Retrieve(ctx, hash)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestRetrieve_ReaderIsClosable(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	hash, err := store.Store(ctx, strings.NewReader("closable"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	rc, err := store.Retrieve(ctx, hash)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestRetrieve_UnknownHash_ErrNotFound(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	var unknown blobstore.Hash32
	copy(unknown[:], bytes.Repeat([]byte{0xde}, 32))

	_, err := store.Retrieve(ctx, unknown)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRetrieve_StagingFile_NotReturned(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	hash, err := store.Store(ctx, strings.NewReader("staging probe"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Plant a _tmp_ file in the root — Retrieve must ignore it.
	tmpPath := filepath.Join(dir, "_tmp_fakeuuid")
	if err := os.WriteFile(tmpPath, []byte("impostor"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rc, err := store.Retrieve(ctx, hash)
	if err != nil {
		t.Fatalf("Retrieve after staging file present: %v", err)
	}
	rc.Close()

	var h blobstore.Hash32
	copy(h[:], bytes.Repeat([]byte{0xab}, 32))
	_, err = store.Retrieve(ctx, h)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unstaged hash, got %v", err)
	}
}

func TestStore_NoDedupByDefault_WritesNewSlot(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir) // Deduplicate defaults to false
	ctx := context.Background()

	content := "no dedup default"
	hash, err := store.Store(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatalf("first Store: %v", err)
	}
	_, err = store.Store(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatalf("second Store: %v", err)
	}

	// Both slot 0 and slot 1 must exist.
	sh := shard.NewShard(hash, dir)
	if _, err := os.Stat(sh.FilePathAt(0)); err != nil {
		t.Errorf("slot 0 missing: %v", err)
	}
	if _, err := os.Stat(sh.FilePathAt(1)); err != nil {
		t.Errorf("slot 1 missing (no-dedup should write new slot): %v", err)
	}
}

func TestStore_Dedup_SkipsSecondWrite(t *testing.T) {
	dir := t.TempDir()
	store := &blobfs.FS{RootDir: dir, Deduplicate: true}
	ctx := context.Background()

	content := "dedup content"
	hash, err := store.Store(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatalf("first Store: %v", err)
	}
	hash2, err := store.Store(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatalf("second Store: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hashes differ: %x vs %x", hash, hash2)
	}

	sh := shard.NewShard(hash, dir)
	if _, err := os.Stat(sh.FilePathAt(0)); err != nil {
		t.Errorf("slot 0 missing: %v", err)
	}
	if _, err := os.Stat(sh.FilePathAt(1)); err == nil {
		t.Errorf("slot 1 should not exist in dedup mode")
	}
}

func TestErase_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	hash, err := store.Store(ctx, strings.NewReader("erase me"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := store.Erase(ctx, hash); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	_, err = store.Retrieve(ctx, hash)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("after Erase: expected ErrNotFound, got %v", err)
	}
}

func TestErase_LIFO(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	content := strings.NewReader("lifo content")
	hash, err := store.Store(ctx, content)
	if err != nil {
		t.Fatalf("first Store: %v", err)
	}
	_, err = store.Store(ctx, strings.NewReader("lifo content"))
	if err != nil {
		t.Fatalf("second Store: %v", err)
	}

	// Erase should remove slot 1 (highest), slot 0 still retrievable.
	if err := store.Erase(ctx, hash); err != nil {
		t.Fatalf("first Erase: %v", err)
	}
	rc, err := store.Retrieve(ctx, hash)
	if err != nil {
		t.Fatalf("Retrieve after first Erase: %v", err)
	}
	rc.Close()

	// Second erase removes slot 0.
	if err := store.Erase(ctx, hash); err != nil {
		t.Fatalf("second Erase: %v", err)
	}
	_, err = store.Retrieve(ctx, hash)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("after both erases: expected ErrNotFound, got %v", err)
	}
}

func TestErase_DoubleErase_ErrNotFound(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	hash, err := store.Store(ctx, strings.NewReader("double erase"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := store.Erase(ctx, hash); err != nil {
		t.Fatalf("first Erase: %v", err)
	}

	err = store.Erase(ctx, hash)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("second Erase: expected ErrNotFound, got %v", err)
	}
}

func TestErase_UnknownHash_ErrNotFound(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	var unknown blobstore.Hash32
	copy(unknown[:], bytes.Repeat([]byte{0xcc}, 32))

	err := store.Erase(ctx, unknown)
	if !errors.Is(err, blobstore.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGC_RemovesStagingFiles(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	for _, name := range []string{"_tmp_aaa", "_tmp_bbb"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("leftover"), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	deleted, err := store.GC(ctx)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if deleted != 2 {
		t.Errorf("GC deleted %d files, want 2", deleted)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "_tmp_") {
			t.Errorf("staging file still present after GC: %s", e.Name())
		}
	}
}

func TestGC_RemovesLockFiles(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	p := filepath.Join(dir, "_lock_deadbeef")
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	deleted, err := store.GC(ctx)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if deleted != 1 {
		t.Errorf("GC deleted %d files, want 1", deleted)
	}
}

func TestGC_PreservesCommittedBlobs(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	hash, err := store.Store(ctx, strings.NewReader("keep me"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	deleted, err := store.GC(ctx)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if deleted != 0 {
		t.Errorf("GC deleted %d files, want 0", deleted)
	}

	rc, err := store.Retrieve(ctx, hash)
	if err != nil {
		t.Fatalf("Retrieve after GC: %v", err)
	}
	rc.Close()
}

func TestGC_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	store := blobfs.NewFS(dir)
	ctx := context.Background()

	deleted, err := store.GC(ctx)
	if err != nil {
		t.Fatalf("GC on empty store: %v", err)
	}
	if deleted != 0 {
		t.Errorf("GC deleted %d files from empty store, want 0", deleted)
	}
}
