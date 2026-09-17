package shard

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gjantsch/stowage/pkg/blobstore"
)

const (
	DirChars = 2
	DirDepth = 2
	MaxSlots = 1000
)

type Shard struct {
	hashString string
	RootDir    string
}

func NewShard(hash blobstore.Hash32, rootDir string) *Shard {
	return &Shard{
		hashString: hex.EncodeToString(hash[:]),
		RootDir:    rootDir,
	}
}

func (s *Shard) Path() string {
	parts := [DirDepth]string{}
	root := s.RootDir
	pathSeparator := string(os.PathSeparator)

	if len(root) > 0 && !strings.HasSuffix(root, pathSeparator) {
		root += pathSeparator
	}

	for i := range DirDepth {
		parts[i] = s.hashString[i*DirChars : (i+1)*DirChars]
	}
	return filepath.Join(root, strings.Join(parts[:], pathSeparator))
}

// FilePathAt returns the path for slot n: <root>/xx/yy/<hex>.<NNN>.bin
// n must be in [0, MaxSlots).
func (s *Shard) FilePathAt(n int) string {
	return filepath.Join(s.Path(), fmt.Sprintf("%s.%03d.bin", s.hashString, n))
}

// LockPath returns the path of the advisory lock file for this hash.
func (s *Shard) LockPath() string {
	return filepath.Join(s.Path(), "_lock_"+s.hashString)
}

// NextSlot scans the shard directory and returns the lowest slot number n such
// that FilePathAt(n) does not exist. Returns (n, false) normally, or
// (-1, true) when all MaxSlots are occupied.
func (s *Shard) NextSlot() (int, bool) {
	for n := range MaxSlots {
		if _, err := os.Stat(s.FilePathAt(n)); os.IsNotExist(err) {
			return n, false
		}
	}
	return -1, true
}

// HighestSlot scans the shard directory and returns the highest occupied slot
// number. Returns (-1, false) when no slot exists.
func (s *Shard) HighestSlot() (int, bool) {
	highest := -1
	for n := range MaxSlots {
		if _, err := os.Stat(s.FilePathAt(n)); err == nil {
			highest = n
		}
	}
	if highest == -1 {
		return -1, false
	}
	return highest, true
}
