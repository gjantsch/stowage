package gc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gjantsch/stowage/internal/blobstore/gc"
)

func TestSweep_RemovesTmpFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	names := []string{"_tmp_one", "_tmp_two", "_tmp_three"}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := gc.Sweep(ctx, dir)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != len(names) {
		t.Errorf("deleted %d, want %d", deleted, len(names))
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("dir not empty after sweep: %v", entries)
	}
}

func TestSweep_PreservesNonTmpFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	keep := filepath.Join(dir, "abc123.bin")
	if err := os.WriteFile(keep, []byte("blob"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_tmp_gone"), []byte("tmp"), 0644); err != nil {
		t.Fatal(err)
	}

	deleted, err := gc.Sweep(ctx, dir)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted %d, want 1", deleted)
	}

	if _, err := os.Stat(keep); err != nil {
		t.Errorf("committed blob removed by GC: %v", err)
	}
}

func TestSweep_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	sub := filepath.Join(dir, "ab", "cd")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "_tmp_nested"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "committed.bin"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}

	deleted, err := gc.Sweep(ctx, dir)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted %d, want 1", deleted)
	}

	if _, err := os.Stat(filepath.Join(sub, "committed.bin")); err != nil {
		t.Errorf("committed blob removed: %v", err)
	}
}

func TestSweep_RemovesLockFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	keep := filepath.Join(dir, "abc123.000.bin")
	if err := os.WriteFile(keep, []byte("blob"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_lock_abc123"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	deleted, err := gc.Sweep(ctx, dir)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted %d, want 1", deleted)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("committed blob removed by GC: %v", err)
	}
}

func TestSweep_Empty(t *testing.T) {
	dir := t.TempDir()
	deleted, err := gc.Sweep(context.Background(), dir)
	if err != nil {
		t.Fatalf("Sweep on empty dir: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted %d on empty dir, want 0", deleted)
	}
}
