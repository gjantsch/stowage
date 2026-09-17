package shard

import (
	"encoding/hex"
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
		{
			name:    "empty root dir",
			rootDir: "",
			want:    "01/23",
		},
		{
			name:    "root dir without trailing slash",
			rootDir: "/data",
			want:    "/data/01/23",
		},
		{
			name:    "root dir with trailing slash",
			rootDir: "/data/",
			want:    "/data/01/23",
		},
		{
			name:    "relative root dir",
			rootDir: "data/",
			want:    "data/01/23",
		},
		{
			name:    "slash only root dir",
			rootDir: "/",
			want:    "/01/23",
		},
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

func TestFilePath(t *testing.T) {
	tests := []struct {
		name    string
		rootDir string
		want    string
	}{
		{
			name:    "empty root dir",
			rootDir: "",
			want:    "01/23/" + testHex + ".bin",
		},
		{
			name:    "root dir without trailing slash",
			rootDir: "/data",
			want:    "/data/01/23/" + testHex + ".bin",
		},
		{
			name:    "root dir with trailing slash",
			rootDir: "/data/",
			want:    "/data/01/23/" + testHex + ".bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewShard(hashFromHex(testHex), tt.rootDir)
			if got := s.FilePath(); got != tt.want {
				t.Errorf("FilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilePath_NoCollision(t *testing.T) {
	hashA := hashFromHex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	hashB := hashFromHex("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	if NewShard(hashA, "/data").FilePath() == NewShard(hashB, "/data").FilePath() {
		t.Error("collision: two different hashes produced the same file path")
	}
}
