package fs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gjantsch/stowage/internal/blobstore/gc"
	"github.com/gjantsch/stowage/internal/blobstore/shard"
	"github.com/gjantsch/stowage/internal/blobstore/staging"
	"github.com/gjantsch/stowage/pkg/blobstore"
)

type FS struct {
	RootDir string
}

func NewFS(rootDir string) *FS {
	return &FS{RootDir: rootDir}
}

func (f *FS) Store(ctx context.Context, r io.Reader) (blobstore.Hash32, error) {
	stager := staging.NewStager(f.RootDir, f.RootDir)
	hash, err := stager.Stage(r, sha256.New())
	if err != nil {
		slog.ErrorContext(ctx, "store: failed",
			"root", f.RootDir,
			"error", err,
		)
		return blobstore.Hash32{}, err
	}
	path := shard.NewShard(hash, f.RootDir).FilePath()
	info, statErr := os.Stat(path)
	size := int64(-1)
	if statErr == nil {
		size = info.Size()
	}
	slog.InfoContext(ctx, "store: ok",
		"hash", hex.EncodeToString(hash[:]),
		"path", path,
		"size", size,
	)
	return hash, nil
}

func (f *FS) Retrieve(ctx context.Context, hash blobstore.Hash32) (io.ReadCloser, error) {
	path := shard.NewShard(hash, f.RootDir).FilePath()
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.InfoContext(ctx, "retrieve: not found",
				"hash", hex.EncodeToString(hash[:]),
				"path", path,
			)
			return nil, fmt.Errorf("%w", blobstore.ErrNotFound)
		}
		slog.ErrorContext(ctx, "retrieve: failed",
			"hash", hex.EncodeToString(hash[:]),
			"path", path,
			"error", err,
		)
		return nil, err
	}
	info, statErr := file.Stat()
	size := int64(-1)
	if statErr == nil {
		size = info.Size()
	}
	slog.InfoContext(ctx, "retrieve: ok",
		"hash", hex.EncodeToString(hash[:]),
		"path", path,
		"size", size,
	)
	return file, nil
}

func (f *FS) Erase(ctx context.Context, hash blobstore.Hash32) error {
	path := shard.NewShard(hash, f.RootDir).FilePath()
	err := os.Remove(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.InfoContext(ctx, "erase: not found",
				"hash", hex.EncodeToString(hash[:]),
				"path", path,
			)
			return fmt.Errorf("%w", blobstore.ErrNotFound)
		}
		slog.ErrorContext(ctx, "erase: failed",
			"hash", hex.EncodeToString(hash[:]),
			"path", path,
			"error", err,
		)
		return err
	}
	slog.InfoContext(ctx, "erase: ok",
		"hash", hex.EncodeToString(hash[:]),
		"path", path,
	)
	return nil
}

func (f *FS) GC(ctx context.Context) (int, error) {
	deleted, err := gc.Sweep(ctx, f.RootDir)
	if err != nil {
		slog.ErrorContext(ctx, "gc: sweep failed",
			"root", f.RootDir,
			"deleted", deleted,
			"error", err,
		)
		return deleted, err
	}
	slog.InfoContext(ctx, "gc: sweep ok",
		"root", f.RootDir,
		"deleted", deleted,
	)
	return deleted, nil
}
