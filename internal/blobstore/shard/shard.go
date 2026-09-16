package shard

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/gjantsch/stowage/pkg/blobstore"
)

const (
	DirChars  = 2 // hex characters per directory level
	DirDepth  = 2
)

type Shard struct {
	Hash    blobstore.Hash32
	RootDir string
}

func NewShard(hash blobstore.Hash32, rootDir string) *Shard {
	return &Shard{
		Hash:    hash,
		RootDir: rootDir,
	}
}

func (s *Shard) Path() string {
	parts := [DirDepth]string{}
	root := s.RootDir
	pathSeparator := string(os.PathSeparator)
	encoded := hex.EncodeToString(s.Hash[:])

	if len(root) > 0 && !strings.HasSuffix(root, pathSeparator) {
		root += pathSeparator
	}

	for i := range DirDepth {
		parts[i] = encoded[i*DirChars : (i+1)*DirChars]
	}
	return filepath.Join(root, strings.Join(parts[:], pathSeparator), encoded+".bin")
}
