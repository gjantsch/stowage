package shard

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gjantsch/stowage/pkg/blobstore"
)

func hashFromHex(s string) blobstore.Hash32 {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		panic("invalid test hash: " + s)
	}
	var h blobstore.Hash32
	copy(h[:], b)
	return h
}

const testHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPath(t *testing.T) {
	tests := []struct {
		name    string
		rootDir string
		want    string
	}{
		{"empty root dir", "", "01/23"},
		{"root dir without trailing slash", "/data", "/data/01/23"},
		{"root dir with trailing slash", "/data/", "/data/01/23"},
		{"relative root dir", "data/", "data/01/23"},
		{"slash only root dir", "/", "/01/23"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewShard(hashFromHex(testHex), tt.rootDir)
			if got := s.Path(); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPath_NoCollision(t *testing.T) {
	hashA := hashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := hashFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	if NewShard(hashA, "/data").Path() == NewShard(hashB, "/data").Path() {
		t.Error("collision: two different hashes produced the same path")
	}
}

func TestFilePathAt(t *testing.T) {
	tests := []struct {
		name    string
		rootDir string
		slot    int
		want    string
	}{
		{"slot 0 empty root", "", 0, "01/23/" + testHex + ".000.bin"},
		{"slot 0 with root", "/data", 0, "/data/01/23/" + testHex + ".000.bin"},
		{"slot 1", "/data", 1, "/data/01/23/" + testHex + ".001.bin"},
		{"slot 42", "/data", 42, "/data/01/23/" + testHex + ".042.bin"},
		{"slot 999", "/data", 999, "/data/01/23/" + testHex + ".999.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewShard(hashFromHex(testHex), tt.rootDir)
			if got := s.FilePathAt(tt.slot); got != tt.want {
				t.Errorf("FilePathAt(%d) = %q, want %q", tt.slot, got, tt.want)
			}
		})
	}
}

func TestFilePathAt_NoCollision(t *testing.T) {
	hashA := hashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := hashFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	if NewShard(hashA, "/data").FilePathAt(0) == NewShard(hashB, "/data").FilePathAt(0) {
		t.Error("collision: two different hashes produced the same file path")
	}
}

func TestLockPath(t *testing.T) {
	s := NewShard(hashFromHex(testHex), "/data")
	want := "/data/01/23/_lock_" + testHex
	if got := s.LockPath(); got != want {
		t.Errorf("LockPath() = %q, want %q", got, want)
	}
}

func TestNextSlot_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewShard(hashFromHex(testHex), dir)
	if err := os.MkdirAll(s.Path(), 0755); err != nil {
		t.Fatal(err)
	}

	n, full := s.NextSlot()
	if full {
		t.Fatal("NextSlot reported full on empty dir")
	}
	if n != 0 {
		t.Errorf("NextSlot() = %d, want 0", n)
	}
}

func TestNextSlot_WithExisting(t *testing.T) {
	dir := t.TempDir()
	s := NewShard(hashFromHex(testHex), dir)
	if err := os.MkdirAll(s.Path(), 0755); err != nil {
		t.Fatal(err)
	}

	// Create slots 0 and 1.
	for _, slot := range []int{0, 1} {
		if err := os.WriteFile(s.FilePathAt(slot), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	n, full := s.NextSlot()
	if full {
		t.Fatal("NextSlot reported full unexpectedly")
	}
	if n != 2 {
		t.Errorf("NextSlot() = %d, want 2", n)
	}
}

func TestHighestSlot_Empty(t *testing.T) {
	dir := t.TempDir()
	s := NewShard(hashFromHex(testHex), dir)
	if err := os.MkdirAll(s.Path(), 0755); err != nil {
		t.Fatal(err)
	}

	n, ok := s.HighestSlot()
	if ok {
		t.Errorf("HighestSlot() reported ok=true on empty dir, n=%d", n)
	}
}

func TestHighestSlot_WithSlots(t *testing.T) {
	dir := t.TempDir()
	s := NewShard(hashFromHex(testHex), dir)
	if err := os.MkdirAll(s.Path(), 0755); err != nil {
		t.Fatal(err)
	}

	for _, slot := range []int{0, 1, 2} {
		if err := os.WriteFile(s.FilePathAt(slot), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	n, ok := s.HighestSlot()
	if !ok {
		t.Fatal("HighestSlot() ok=false unexpectedly")
	}
	if n != 2 {
		t.Errorf("HighestSlot() = %d, want 2", n)
	}
}

func TestNextSlot_FilePathAt_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	s := NewShard(hashFromHex(testHex), dir)
	if err := os.MkdirAll(s.Path(), 0755); err != nil {
		t.Fatal(err)
	}

	for expected := range 5 {
		n, full := s.NextSlot()
		if full {
			t.Fatalf("slot %d: NextSlot reported full", expected)
		}
		if n != expected {
			t.Fatalf("slot %d: NextSlot() = %d", expected, n)
		}
		p := s.FilePathAt(n)
		if !filepath.IsAbs(p) && dir != "" {
			t.Errorf("FilePathAt(%d) = %q is not absolute", n, p)
		}
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatalf("WriteFile slot %d: %v", n, err)
		}
	}
}
