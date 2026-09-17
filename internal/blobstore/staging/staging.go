package staging

import (
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/pkg/blobstore"
	"github.com/google/uuid"
)

var errSlotsFull = errors.New("stowage: all 1000 slots occupied for hash")

type Stager struct {
	stageDir    string
	uuid        string
	rootPath    string
	deduplicate bool
}

func NewStager(stageDir, rootPath string, deduplicate bool) *Stager {
	return &Stager{
		stageDir:    stageDir,
		uuid:        uuid.New().String(),
		rootPath:    rootPath,
		deduplicate: deduplicate,
	}
}

func (s *Stager) TempFile() string {
	return filepath.Join(s.stageDir, "_tmp_"+s.uuid)
}

func (s *Stager) Stage(r io.Reader, h hash.Hash) (blobstore.Hash32, error) {
	cleanup := true
	closed := false

	tmpPath := s.TempFile()
	f, err := os.Create(tmpPath)
	if err != nil {
		return blobstore.Hash32{}, err
	}
	defer func() {
		if !closed {
			f.Close()
		}
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	tee := io.TeeReader(r, h)
	if _, err = io.Copy(f, tee); err != nil {
		return blobstore.Hash32{}, err
	}
	if err = f.Close(); err != nil {
		return blobstore.Hash32{}, err
	}
	closed = true

	var hash blobstore.Hash32
	copy(hash[:], h.Sum(nil))

	sh := shard.NewShard(hash, s.rootPath)
	if err = os.MkdirAll(sh.Path(), 0755); err != nil {
		return blobstore.Hash32{}, err
	}

	// Dedup early-exit: if slot 0 exists the content is already committed.
	// Check before the lock — the decision is final once slot 0 is present.
	if s.deduplicate {
		if _, statErr := os.Stat(sh.FilePathAt(0)); statErr == nil {
			return hash, nil
		}
	}

	// Acquire advisory lock: O_CREATE|O_EXCL is atomic on POSIX and Windows.
	// If another process holds the lock it is writing the same content — the
	// result will be identical, so we can safely skip staging and return.
	lockPath := sh.LockPath()
	lf, lockErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if lockErr != nil {
		if errors.Is(lockErr, os.ErrExist) {
			return hash, nil
		}
		return blobstore.Hash32{}, lockErr
	}
	lf.Close()
	defer os.Remove(lockPath)

	// Find the next available slot.
	nextSlot, full := sh.NextSlot()
	if full {
		return blobstore.Hash32{}, errSlotsFull
	}

	cleanup = false
	if err = os.Rename(tmpPath, sh.FilePathAt(nextSlot)); err != nil {
		cleanup = true
		return blobstore.Hash32{}, err
	}

	return hash, nil
}
