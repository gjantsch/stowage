package staging

import (
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/pkg/blobstore"
	"github.com/google/uuid"
)

type Stager struct {
	stageDir string
	uuid     string
	rootPath string
}

func NewStager(stageDir, rootPath string) *Stager {
	return &Stager{
		stageDir: stageDir,
		uuid:     uuid.New().String(),
		rootPath: rootPath,
	}
}

func (s *Stager) TempFile() string {
	return filepath.Join(s.stageDir, "_tmp_"+s.uuid)
}
func (s *Stager) Stage(r io.Reader, h hash.Hash) (blobstore.Hash32, error) {

	// cleanup is not needed if the staging and renaming succeed
	cleanup := true
	// close the tmp file only if it hasn't been closed normally
	closed := false

	// create the temporary file
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

	_, err = io.Copy(f, tee)
	if err != nil {
		return blobstore.Hash32{}, err
	}

	if err = f.Close(); err != nil {
		return blobstore.Hash32{}, err
	}
	closed = true

	// compute the final hash value
	var hash blobstore.Hash32
	copy(hash[:], h.Sum(nil))

	// create the shard to get the final storage path
	shard := shard.NewShard(hash, s.rootPath)
	err = os.MkdirAll(shard.Path(), 0755)
	if err != nil {
		return blobstore.Hash32{}, err
	}

	// everything succeeded, so we don't need
	// to clean up the temporary file anymore
	cleanup = false

	// move the temporary file to its final location
	err = os.Rename(tmpPath, shard.FilePath())
	if err != nil {
		return blobstore.Hash32{}, err
	}

	// return the final hash value
	return hash, nil
}
